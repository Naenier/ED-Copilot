"use strict";
// Optional child-window sizing utilities; no application state is stored here.
const GALAXY_MIN = Object.freeze({ w: 900, h: 560 });
const finite = value => typeof value === "number" && Number.isFinite(value);
function childWindowKind(value) {
  let address;
  try { address = new URL(value); } catch { return null; }
  const route = address.pathname.replace(/\/+$/, "");
  if (route === "/galaxy" || route === "/galaxy.html") return "galaxy";
  if (route === "/colonize" || route === "/colonize.html") return "colonize";
  return null;
}
function galaxyWindowBounds(saved = {}, area = { x: 0, y: 0, width: 1280, height: 800 }) {
  const width = Math.min(area.width, Math.max(Math.min(GALAXY_MIN.w, area.width), finite(saved?.width) ? Math.round(saved.width) : Math.round(area.width * 0.9)));
  const height = Math.min(area.height, Math.max(Math.min(GALAXY_MIN.h, area.height), finite(saved?.height) ? Math.round(saved.height) : Math.round(area.height * 0.9)));
  return {
    width, height,
    x: Math.round(Math.max(area.x, Math.min(area.x + area.width - width, finite(saved?.x) ? saved.x : area.x + (area.width - width) / 2))),
    y: Math.round(Math.max(area.y, Math.min(area.y + area.height - height, finite(saved?.y) ? saved.y : area.y + (area.height - height) / 2))),
  };
}
module.exports = { GALAXY_MIN, childWindowKind, galaxyWindowBounds };

