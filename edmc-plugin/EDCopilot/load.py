"""EDMC -> ED Copilot loopback bridge. Does not publish to Internet services."""

import copy
import datetime
import logging
import queue
import re
import threading
import time

import requests
import tkinter as tk
import myNotebook as nb
from config import appname, config
from monitor import monitor

PLUGIN_VERSION = "1.0.0"
ENDPOINT = "http://127.0.0.1:7122/api/integrations/edmc"
logger = logging.getLogger(f"{appname}.EDCopilot")
_jobs = queue.Queue(maxsize=128)
_stop = threading.Event()
_lock = threading.Lock()
_thread = None
_enabled = False
_token = ""
_commander = ""
_beta = False
_enabled_var = None
_token_var = None

# Exact public fields only, including nested records. No balance, mission,
# reputation, private carrier warehouse, credentials or unfiltered CAPI profile.
_SHAPES = {
    "StarSystem": str, "SystemAddress": int, "StarPos": [float],
    "Body": str, "BodyID": int, "BodyType": str, "BodyName": str,
    "StationName": str, "StationType": str, "MarketID": int,
    "StationEconomy": str, "StationGovernment": str,
    "StationAllegiance": str, "StationServices": [str],
    "StationFaction": {"Name": str, "FactionState": str},
    "StationEconomies": [{"Name": str, "Proportion": float}],
    "SystemAllegiance": str, "SystemEconomy": str, "SystemGovernment": str,
    "SystemSecurity": str, "Population": int,
    "SystemFaction": {"Name": str, "FactionState": str},
    "ScanType": str, "PlanetClass": str, "StarType": str,
    "Atmosphere": str, "Volcanism": str, "Landable": bool,
    "SurfaceTemperature": float, "SurfaceGravity": float,
    "ReserveLevel": str,
    "Rings": [{"Name": str, "RingClass": str, "MassMT": float, "InnerRad": float, "OuterRad": float}],
    "Materials": [{"Name": str, "Percent": float}],
    "Signals": [{"Type": str, "Count": int}],
    "Horizons": bool, "Odyssey": bool,
}
_EVENTS = {
    "Fileheader", "LoadGame", "Location", "FSDJump", "CarrierJump",
    "Docked", "Undocked", "Scan", "SAASignalsFound", "Market",
    "Outfitting", "Shipyard", "Shutdown", "StartUp",
}
_COMMODITY = {
    "name": str, "meanPrice": int, "buyPrice": int, "sellPrice": int,
    "stock": int, "demand": int, "stockBracket": int, "demandBracket": int,
    "categoryname": str, "legality": str, "statusFlags": [str],
}


def _select(value, shape):
    if isinstance(shape, dict):
        if not isinstance(value, dict):
            return None
        result = {}
        for key, child in shape.items():
            parsed = _select(value.get(key), child)
            if parsed is not None:
                result[key] = parsed
        return result
    if isinstance(shape, list):
        if not isinstance(value, list):
            return None
        return [item for item in (_select(item, shape[0]) for item in value[:10000]) if item is not None]
    if shape is float:
        return value if type(value) in (int, float) else None
    if shape is str:
        return value[:8192] if isinstance(value, str) else None
    return value if type(value) is shape else None


def _settings():
    global _enabled, _token
    token = config.get_str("EDCopilotBridgeToken") or ""
    enabled = bool(config.get_bool("EDCopilotBridgeEnabled"))
    with _lock:
        _token = token.strip() if re.fullmatch(r"[A-Za-z0-9_\-=+/]{24,512}", token.strip()) else ""
        _enabled = enabled and bool(_token)


def _put(payload):
    with _lock:
        enabled = _enabled
    if not enabled:
        return
    try:
        _jobs.put_nowait(copy.deepcopy(payload))
    except queue.Full:
        # Local preview is transient. Never block EDMC or keep an unbounded log.
        pass


def _worker():
    session = requests.Session()
    session.trust_env = False  # Do not send loopback data through a user proxy.
    last_warning = 0.0
    while not _stop.is_set():
        try:
            payload = _jobs.get(timeout=15)
        except queue.Empty:
            payload = {"type": "heartbeat"}
        if payload is None or _stop.is_set():
            break
        with _lock:
            enabled, token, commander, beta = _enabled, _token, _commander, _beta
        if not enabled:
            continue
        payload.setdefault("commander", commander)
        payload.setdefault("isBeta", beta)
        payload["pluginVersion"] = PLUGIN_VERSION
        payload["observedAt"] = datetime.datetime.now(datetime.timezone.utc).isoformat()
        try:
            response = session.post(
                ENDPOINT, json=payload,
                headers={"X-EDCopilot-Token": token, "User-Agent": f"EDCopilot-EDMC-Bridge/{PLUGIN_VERSION}"},
                timeout=(1, 2), allow_redirects=False,
            )
            if response.status_code != 200 and time.monotonic() - last_warning > 60:
                logger.warning("ED Copilot bridge: HTTP %s; check pairing token and integration mode", response.status_code)
                last_warning = time.monotonic()
            response.close()
        except requests.RequestException:
            if time.monotonic() - last_warning > 60:
                logger.info("ED Copilot bridge: local application is unavailable")
                last_warning = time.monotonic()
    session.close()


def plugin_start3(plugin_dir):
    global _thread
    _settings()
    _stop.clear()
    _thread = threading.Thread(target=_worker, name="EDCopilotBridge", daemon=True)
    _thread.start()
    _put({"type": "heartbeat"})
    return "ED Copilot"


def plugin_prefs(parent, cmdr, is_beta):
    global _enabled_var, _token_var
    frame = nb.Frame(parent)
    _enabled_var = tk.BooleanVar(value=config.get_bool("EDCopilotBridgeEnabled"))
    _token_var = tk.StringVar(value=config.get_str("EDCopilotBridgeToken") or "")
    nb.Checkbutton(frame, text="Связать с ED Copilot на этом компьютере", variable=_enabled_var).grid(row=0, column=0, columnspan=2, sticky=tk.W)
    nb.Label(frame, text="Код сопряжения:").grid(row=1, column=0, sticky=tk.W)
    nb.Entry(frame, textvariable=_token_var, show="*", width=48).grid(row=1, column=1, sticky=tk.EW)
    nb.Label(frame, text="Код: ED Copilot → Интеграции → EDMC. Адрес: 127.0.0.1:7122.").grid(row=2, column=0, columnspan=2, sticky=tk.W)
    nb.Label(frame, text="В режиме EDMC отправку в Inara и EDDN выполняют плагины EDMC.").grid(row=3, column=0, columnspan=2, sticky=tk.W)
    frame.columnconfigure(1, weight=1)
    return frame


def prefs_changed(cmdr, is_beta):
    if _enabled_var is not None:
        config.set("EDCopilotBridgeEnabled", _enabled_var.get())
    if _token_var is not None:
        config.set("EDCopilotBridgeToken", _token_var.get().strip())
    _settings()
    _put({"type": "heartbeat", "commander": cmdr or "", "isBeta": bool(is_beta)})


def journal_entry(cmdr, is_beta, system, station, entry, state):
    global _commander, _beta
    with _lock:
        _commander, _beta = cmdr or "", bool(is_beta)
    if entry.get("event") not in _EVENTS:
        return None
    filtered = _select(entry, _SHAPES)
    filtered["event"] = entry.get("event")
    if isinstance(entry.get("timestamp"), str):
        filtered["timestamp"] = entry["timestamp"]
    try:
        live_galaxy = monitor.is_live_galaxy()
    except Exception:
        live_galaxy = False
    _put({
        "type": "journal", "commander": cmdr or "", "isBeta": bool(is_beta),
        "liveGalaxy": live_galaxy, "system": system, "station": station,
        "gameVersion": state.get("GameVersion"), "gameBuild": state.get("GameBuild"),
        "entry": filtered,
    })
    return None


def cmdr_data(data, is_beta):
    # These are locally forwarded public observations, not the OAuth credential
    # or the commander's complete CAPI profile. EDMC owns external publishing.
    raw = getattr(data, "data", data)
    if not isinstance(raw, dict):
        return None
    safe = _select(raw, {
        "commander": {"name": str, "docked": bool},
        "lastSystem": {"name": str},
        "lastStarport": {
            "id": int, "name": str, "commodities": [_COMMODITY],
            "economies": [{"name": str, "proportion": float}],
        },
    })
    _put({"type": "capi", "commander": safe.get("commander", {}).get("name", ""), "isBeta": bool(is_beta), "data": safe})
    return None


def plugin_stop():
    _stop.set()
    try:
        _jobs.put_nowait(None)
    except queue.Full:
        pass
    if _thread is not None:
        _thread.join(timeout=4)
