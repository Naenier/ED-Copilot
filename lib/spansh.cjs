"use strict";

const { getResource, normalize } = require("./catalog.cjs");

const MAX_BODIES = 4096;
const MAX_SIGNALS = 256;

function finite(value) {
  if (value == null || value === "" || typeof value === "boolean") return null;
  const number = Number(value);
  return Number.isFinite(number) ? number : null;
}

function signalRows(value) {
  const payload = value?.signals ?? value;
  if (Array.isArray(payload)) {
    return payload.slice(0, MAX_SIGNALS).flatMap((signal) => {
      const type = signal?.Type ?? signal?.name ?? signal?.type ?? signal?.key;
      const count = finite(signal?.Count ?? signal?.count);
      return typeof type === "string" && count > 0 ? [{ Type: type, Count: Math.floor(count) }] : [];
    });
  }
  if (!payload || typeof payload !== "object") return [];
  return Object.entries(payload).slice(0, MAX_SIGNALS).flatMap(([type, rawCount]) => {
    const count = finite(rawCount);
    return count > 0 ? [{ Type: type, Count: Math.floor(count) }] : [];
  });
}

function materialRows(value) {
  const entries = Array.isArray(value)
    ? value.map((entry) => [entry?.Name ?? entry?.name, entry?.Percent ?? entry?.share ?? entry?.percent])
    : Object.entries(value && typeof value === "object" ? value : {});
  return entries.slice(0, 128).flatMap(([name, rawPercent]) => {
    const percent = finite(rawPercent);
    const key = typeof name === "string" ? getResource(name)?.key ?? normalize(name) : "";
    return key && percent != null && percent >= 0 && percent <= 100 ? [{ key, percent }] : [];
  });
}

function spanshSystemFromDump(payload) {
  const system = payload?.system;
  if (!system || typeof system.name !== "string" || !Array.isArray(system.bodies)) {
    throw new Error("Spansh вернул ответ без данных системы.");
  }
  const coords = [system.coords?.x, system.coords?.y, system.coords?.z].map(finite);
  return {
    address: system.id64 ?? null,
    name: system.name,
    coordinates: coords.every((value) => value != null) ? coords : null,
    bodies: system.bodies.slice(0, MAX_BODIES).filter((body) => typeof body?.name === "string").map((body) => ({
      id: finite(body.bodyId) ?? body.name,
      name: body.name,
      source: "spansh",
      timestamp: typeof body.updateTime === "string" ? body.updateTime : null,
      landable: body.isLandable === true,
      planetClass: body.subType ?? "",
      distanceLs: finite(body.distanceToArrival),
      gravity: finite(body.gravity),
      reserve: body.reserveLevel ?? "",
      materials: materialRows(body.materials),
      rings: (Array.isArray(body.rings) ? body.rings : []).slice(0, 64)
        .filter((ring) => typeof ring?.name === "string")
        .map((ring) => ({
          name: ring.name,
          type: ring.type ?? "",
          signals: signalRows(ring.signals).map((signal) => ({
            key: getResource(signal.Type)?.key ?? normalize(signal.Type),
            name: signal.Type,
            count: signal.Count,
          })),
        })),
      signals: signalRows(body.signals),
    })),
  };
}

module.exports = { spanshSystemFromDump };
