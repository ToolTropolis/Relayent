"""
Primary author: Navjyot Nishant
Created on: 2026-07-16
Last updated: 2026-07-16
Description: Python reference client for the Relayent /v1 API. Optional convenience
  over the language-neutral HTTP contract (see openapi.yaml) — any app may call the
  raw endpoints instead. Consuming apps depend only on the /v1 API, never on the
  relay/bridge implementation.
AI usage: Built with assistance from AI tools for implementation acceleration,
  review, and refactoring.
"""

from __future__ import annotations

import time
from typing import Any, Optional

import requests


class RelayentError(Exception):
    """Base error for Relayent client failures."""


class BridgeOfflineError(RelayentError):
    """Raised when no bridge is polling the relay for this pairing key."""


class JobFailedError(RelayentError):
    """Raised when the bridge reports the job failed (e.g. CLI error)."""


class RelayentClient:
    """Thin client for the Relayent relay.

    Example:
        client = RelayentClient("https://relay.example.com", pairing_key="...")
        result = client.run(
            backend="claude",
            prompt="Return {\\"ok\\": true} as JSON",
            json_schema={"type": "object", "properties": {"ok": {"type": "boolean"}}},
        )
    """

    def __init__(
        self,
        relay_url: str,
        pairing_key: str,
        *,
        request_timeout: float = 15.0,
        poll_timeout: float = 120.0,
    ):
        if not relay_url:
            raise ValueError("relay_url is required")
        if not pairing_key:
            raise ValueError("pairing_key is required")
        self.relay_url = relay_url.rstrip("/")
        self.pairing_key = pairing_key
        self.request_timeout = request_timeout
        self.poll_timeout = poll_timeout

    # --- public API ---

    def bridge_online(self) -> bool:
        """Return True if a bridge is currently polling for this pairing key."""
        r = self._get("/v1/bridge/online", timeout=self.request_timeout)
        return bool(r.json().get("online"))

    def run(
        self,
        backend: str,
        prompt: str,
        *,
        model: Optional[str] = None,
        system: Optional[str] = None,
        json_schema: Optional[dict] = None,
        require_online: bool = True,
    ) -> Any:
        """Enqueue a job and block until the bridge returns a result.

        Returns the parsed JSON object when `json_schema` is given and the CLI
        produced valid JSON, otherwise the raw text string.

        Raises BridgeOfflineError if `require_online` and no bridge is polling,
        or if the wait elapses with the job still pending. Raises JobFailedError
        if the bridge reports an error.
        """
        if require_online and not self.bridge_online():
            raise BridgeOfflineError(
                "Relayent bridge is offline — start your Relayent bridge or switch AI provider"
            )

        body: dict[str, Any] = {"backend": backend, "prompt": prompt}
        if model:
            body["model"] = model
        if system:
            body["system"] = system
        if json_schema is not None:
            body["json_schema"] = json_schema

        enq = self._post("/v1/jobs", json=body, timeout=self.request_timeout).json()
        job_id = enq["job_id"]

        result = self._wait_for_result(job_id)
        status = result.get("status")
        if status == "done":
            if result.get("json") is not None:
                return result["json"]
            return result.get("text", "")
        if status == "error":
            raise JobFailedError(result.get("error") or "Relayent job failed")
        # Still pending after the wait budget: treat as offline/unresponsive.
        raise BridgeOfflineError(
            "Relayent job did not complete — the bridge may be offline or overloaded"
        )

    # --- internals ---

    def _wait_for_result(self, job_id: str) -> dict:
        """Long-poll GET /v1/jobs/{id} until done/error or the poll budget elapses."""
        deadline = time.monotonic() + self.poll_timeout
        while True:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                # One last non-blocking check before giving up.
                return self._get(
                    f"/v1/jobs/{job_id}", timeout=self.request_timeout
                ).json()
            # The relay caps its own blocking wait; loop covers longer budgets.
            r = self._get(
                f"/v1/jobs/{job_id}?wait=1",
                timeout=min(self.request_timeout + self.poll_timeout, remaining + 10),
            )
            data = r.json()
            if data.get("status") in ("done", "error"):
                return data

    def _headers(self) -> dict:
        return {"Authorization": f"Bearer {self.pairing_key}"}

    def _get(self, path: str, timeout: float) -> requests.Response:
        r = requests.get(self.relay_url + path, headers=self._headers(), timeout=timeout)
        self._raise_for_status(r)
        return r

    def _post(self, path: str, json: dict, timeout: float) -> requests.Response:
        r = requests.post(
            self.relay_url + path, headers=self._headers(), json=json, timeout=timeout
        )
        self._raise_for_status(r)
        return r

    @staticmethod
    def _raise_for_status(r: requests.Response) -> None:
        if r.status_code >= 400:
            try:
                msg = r.json().get("error", r.text)
            except Exception:
                msg = r.text
            raise RelayentError(f"relay {r.status_code}: {msg}")
