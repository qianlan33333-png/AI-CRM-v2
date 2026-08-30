"""Isolated stdlib-only bridge for the official WeCom Finance SDK."""

from __future__ import annotations

import ctypes
import json
import sys
from pathlib import Path
from typing import Any


RESULT_PREFIX = "AICRM_SDK_RESULT="
PTR = ctypes.c_void_p


class NativeSdkError(RuntimeError):
    def __init__(self, code: str, native_code: int = 0) -> None:
        super().__init__(code)
        self.code = code
        self.native_code = int(native_code)


def _library(path_value: str):
    path = Path(path_value)
    if not path.is_file():
        raise NativeSdkError("sdk_library_missing")
    lib = ctypes.CDLL(str(path))
    lib.NewSdk.argtypes, lib.NewSdk.restype = [], PTR
    lib.Init.argtypes, lib.Init.restype = [PTR, ctypes.c_char_p, ctypes.c_char_p], ctypes.c_int
    lib.DestroySdk.argtypes, lib.DestroySdk.restype = [PTR], None
    lib.NewSlice.argtypes, lib.NewSlice.restype = [], PTR
    lib.FreeSlice.argtypes, lib.FreeSlice.restype = [PTR], None
    lib.GetChatData.argtypes = [PTR, ctypes.c_ulonglong, ctypes.c_uint, ctypes.c_char_p, ctypes.c_char_p, ctypes.c_int, PTR]
    lib.GetChatData.restype = ctypes.c_int
    lib.GetContentFromSlice.argtypes, lib.GetContentFromSlice.restype = [PTR], ctypes.c_char_p
    lib.DecryptData.argtypes, lib.DecryptData.restype = [ctypes.c_char_p, ctypes.c_char_p, PTR], ctypes.c_int
    return lib


def _slice_json(lib, pointer: PTR) -> dict[str, Any]:
    raw = lib.GetContentFromSlice(pointer)
    value = json.loads(raw.decode("utf-8") if raw else "{}")
    if not isinstance(value, dict):
        raise NativeSdkError("sdk_payload_invalid")
    return value


def _fetch(request: dict[str, Any]) -> dict[str, Any]:
    lib = _library(str(request.get("lib_path") or ""))
    sdk = lib.NewSdk()
    if not sdk:
        raise NativeSdkError("sdk_pointer_missing")
    pointer = None
    try:
        result = lib.Init(sdk, str(request.get("corp_id") or "").encode(), str(request.get("archive_secret") or "").encode())
        if result:
            raise NativeSdkError("sdk_init_failed", result)
        pointer = lib.NewSlice()
        if not pointer:
            raise NativeSdkError("sdk_slice_missing")
        result = lib.GetChatData(
            sdk,
            int(request.get("seq") or 0),
            max(1, min(int(request.get("limit") or 100), 1000)),
            None,
            None,
            max(1, int(request.get("timeout") or 60)),
            pointer,
        )
        if result:
            raise NativeSdkError("sdk_fetch_failed", result)
        return {"ok": True, "payload": _slice_json(lib, pointer)}
    finally:
        if pointer:
            lib.FreeSlice(pointer)
        lib.DestroySdk(sdk)


def _decrypt(request: dict[str, Any]) -> dict[str, Any]:
    items = request.get("items")
    if not isinstance(items, list) or len(items) > 1000:
        raise NativeSdkError("sdk_decrypt_items_invalid")
    lib = _library(str(request.get("lib_path") or ""))
    payloads: list[dict[str, Any]] = []
    for item in items:
        if not isinstance(item, dict):
            raise NativeSdkError("sdk_decrypt_item_invalid")
        pointer = lib.NewSlice()
        if not pointer:
            raise NativeSdkError("sdk_slice_missing")
        try:
            result = lib.DecryptData(
                str(item.get("random_key") or "").encode(),
                str(item.get("encrypt_chat_msg") or "").encode(),
                pointer,
            )
            if result:
                raise NativeSdkError("sdk_decrypt_failed", result)
            payloads.append(_slice_json(lib, pointer))
        finally:
            lib.FreeSlice(pointer)
    return {"ok": True, "payloads": payloads}


def _emit(payload: dict[str, Any]) -> None:
    sys.stdout.write(RESULT_PREFIX + json.dumps(payload, ensure_ascii=False, separators=(",", ":")) + "\n")
    sys.stdout.flush()


def main() -> int:
    try:
        request = json.loads(sys.stdin.read() or "{}")
        if not isinstance(request, dict):
            raise NativeSdkError("sdk_request_invalid")
        if len(sys.argv) != 2 or sys.argv[1] not in {"fetch", "decrypt"}:
            raise NativeSdkError("sdk_operation_invalid")
        _emit(_fetch(request) if sys.argv[1] == "fetch" else _decrypt(request))
        return 0
    except NativeSdkError as error:
        _emit({"ok": False, "error_code": error.code, "native_code": error.native_code})
        return 1
    except Exception as error:  # fail closed without emitting provider content
        _emit({"ok": False, "error_code": f"sdk_helper_{type(error).__name__}"})
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
