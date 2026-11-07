"use strict";

// Palette import reads settings only. It never writes to Elite Dangerous or
// evaluates EDHM's INI directives, shader files, expressions, or includes.
const fs = require("node:fs");
const fsp = fs.promises;
const path = require("node:path");
const os = require("node:os");

const IDENTITY = Object.freeze([[1, 0, 0], [0, 1, 0], [0, 0, 1]].map(Object.freeze));
const COLOR_KEYS = Object.freeze(["accent", "text", "muted", "bg", "panel", "good", "warn", "danger"]);
const PRESETS = Object.freeze([
  { id: "blue", name: "Синий космос", colors: { accent: "#28a0f0", text: "#e0e0e0", muted: "#8898a8", bg: "#07101a", panel: "#0a1622", good: "#63d8bd", warn: "#edbd66", danger: "#fa8181" } },
  { id: "orange", name: "Оранжевый HUD", colors: { accent: "#ff8a24", text: "#f0e3d6", muted: "#b7a18d", bg: "#160d07", panel: "#201208", good: "#a2d572", warn: "#ffd36a", danger: "#ff7770" } },
  { id: "green", name: "Изумруд", colors: { accent: "#42d5a4", text: "#e0eee8", muted: "#8eaaa0", bg: "#06130f", panel: "#0a1d17", good: "#83e8ad", warn: "#e7c66a", danger: "#f57f8c" } },
  { id: "purple", name: "Фиолетовый", colors: { accent: "#b78aff", text: "#eae2f4", muted: "#a396b4", bg: "#100a19", panel: "#190f25", good: "#78d8c4", warn: "#eac57a", danger: "#fb87a9" } },
].map(preset => Object.freeze({ ...preset, colors: Object.freeze(preset.colors), matrix: IDENTITY })));
const MAX_FILE_BYTES = 1024 * 1024;
class ThemeError extends Error {
  constructor(message, code = "THEME_INVALID") { super(message); this.name = "ThemeError"; this.code = code; this.status = 400; }
}
const object = value => value && typeof value === "object" && !Array.isArray(value) ? value : {};
const cloneMatrix = value => value.map(row => [...row]);
const clamp = value => Math.max(0, Math.min(1, value));
const safeText = (value, length = 1024) => typeof value === "string" ? value.replace(/[\u0000-\u001f]/g, " ").slice(0, length) : "";
function color(value) {
  if (typeof value !== "string" || !/^#[0-9a-f]{6}$/i.test(value)) throw new ThemeError("Цвет должен быть записан как #RRGGBB.", "COLOR_INVALID");
  return value.toLowerCase();
}
function matrix(value) {
  if (!Array.isArray(value) || value.length !== 3 || value.some(row => !Array.isArray(row) || row.length !== 3 ||
    row.some(n => typeof n !== "number" || !Number.isFinite(n) || n < -2 || n > 2))) {
    throw new ThemeError("Матрица цвета должна содержать девять чисел от −2 до 2.", "MATRIX_INVALID");
  }
  return cloneMatrix(value);
}
function sourceInfo(value) {
  if (!value) return null;
  const item = object(value), result = {
    kind: ["edhm", "xml"].includes(item.kind) ? item.kind : "xml",
    name: safeText(item.name, 200), path: safeText(item.path),
    files: Array.isArray(item.files) ? item.files.filter(v => typeof v === "string").slice(0, 8).map(v => safeText(v)) : [],
    modifiedAt: Number.isFinite(Date.parse(item.modifiedAt)) ? new Date(item.modifiedAt).toISOString() : null,
    importedAt: Number.isFinite(Date.parse(item.importedAt)) ? new Date(item.importedAt).toISOString() : null,
    notes: Array.isArray(item.notes) ? item.notes.filter(v => typeof v === "string").slice(0, 5).map(v => safeText(v, 500)) : [],
  };
  return result;
}
function normalizeTheme(input = {}) {
  const value = object(input);
  const mode = value.mode ?? "preset", preset = value.preset ?? "blue";
  if (!["preset", "custom", "matrix", "game"].includes(mode)) throw new ThemeError("Неизвестный способ настройки палитры.", "MODE_INVALID");
  const base = PRESETS.find(item => item.id === preset);
  if (!base) throw new ThemeError("Неизвестная готовая палитра.", "PRESET_INVALID");
  const colors = { ...base.colors };
  for (const key of COLOR_KEYS) if (object(value.colors)[key] !== undefined) colors[key] = color(value.colors[key]);
  const opacity = value.overlayOpacity ?? 0.82;
  if (typeof opacity !== "number" || !Number.isFinite(opacity) || opacity < 0.4 || opacity > 1) {
    throw new ThemeError("Плотность фона оверлея должна быть от 40 до 100 процентов.", "OPACITY_INVALID");
  }
  return { mode, preset, colors, matrix: matrix(value.matrix ?? IDENTITY), overlayOpacity: Math.round(opacity * 100) / 100,
    source: mode === "game" ? sourceInfo(value.source) : null };
}
function rgb(value) { return [1, 3, 5].map(start => parseInt(value.slice(start, start + 2), 16)); }
function hex(value) { return "#" + value.map(n => Math.round(Math.max(0, Math.min(255, n))).toString(16).padStart(2, "0")).join(""); }
function mix(first, second, amount) { const left = rgb(first), right = rgb(second); return hex(left.map((n, i) => n * (1 - amount) + right[i] * amount)); }
function transform(value, coefficients) {
  const input = rgb(value).map(v => v / 255);
  // Elite's MatrixRed/Green/Blue rows contain each input channel's
  // contribution to the output RGB vector, not one output channel's weights.
  return hex([0, 1, 2].map(output => 255 * clamp(input.reduce((sum, channel, row) => sum + channel * coefficients[row][output], 0))));
}
function matrixColors(coefficients) {
  const accent = transform("#ff7f00", coefficients);
  return { accent, text: "#e0e0e0", muted: mix("#a8b1ba", accent, 0.14),
    bg: mix("#07090c", accent, 0.055), panel: mix("#0a0d12", accent, 0.085),
    good: transform("#63d8bd", coefficients), warn: transform("#edbd66", coefficients), danger: transform("#fa8181", coefficients) };
}
function derivePalette(input = {}) {
  const settings = normalizeTheme(input);
  const values = settings.mode === "preset" ? { ...PRESETS.find(item => item.id === settings.preset).colors } :
    settings.mode === "matrix" ? matrixColors(settings.matrix) : { ...settings.colors };
  return { ...values, accentBright: mix(values.accent, "#ffffff", 0.33), accentDim: mix(values.accent, values.bg, 0.67),
    faint: mix(values.muted, values.bg, 0.40), panelRaised: mix(values.panel, values.accent, 0.07),
    line: mix(values.accent, values.bg, 0.78), overlayOpacity: settings.overlayOpacity };
}
function parseXmlMatrix(raw) {
  if (typeof raw !== "string" || raw.length > MAX_FILE_BYTES) throw new ThemeError("Слишком большой файл цветовой матрицы.", "FILE_TOO_LARGE");
  const text = raw.replace(/<!--[\s\S]*?-->/g, "");
  if (/<!DOCTYPE|<!ENTITY/i.test(text)) throw new ThemeError("Файл матрицы содержит неподдерживаемые XML-объявления.", "XML_UNSUPPORTED");
  const gui = /<GUIColour\b[^>]*>([\s\S]*?)<\/GUIColour>/i.exec(text)?.[1] || text;
  const selected = /<Default\b[^>]*>([\s\S]*?)<\/Default>/i.exec(gui)?.[1] || gui;
  if (!/<Matrix(?:Red|Green|Blue)\b/i.test(selected)) return null;
  const rows = ["Red", "Green", "Blue"].map(name => {
    const body = new RegExp(`<Matrix${name}\\b[^>]*>([^<]+)<\\/Matrix${name}>`, "i").exec(selected)?.[1];
    return body ? body.trim().split(/[\s,]+/).map(Number) : [];
  });
  return matrix(rows);
}
function parseIni(raw) {
  const values = Object.create(null);
  let constants = false;
  for (const source of raw.replace(/^\uFEFF/, "").split(/\r?\n/)) {
    const line = source.split(";", 1)[0].trim();
    const section = /^\[([^\]]+)\]/.exec(line);
    if (section) { constants = section[1].toLowerCase() === "constants"; continue; }
    if (!constants) continue;
    const match = /^\$?([xyzw]\d+)\s*=\s*([-+]?(?:\d+(?:\.\d*)?|\.\d+)(?:e[-+]?\d+)?)\s*$/i.exec(line);
    if (match && Number.isFinite(Number(match[2]))) values[match[1].toLowerCase()] = Number(match[2]);
  }
  return values;
}
function parseEdhmMatrix(raw) {
  const data = parseIni(raw), keys = [150, 151, 152].map(id => ["x", "y", "z"].map(axis => axis + id));
  if (!keys.flat().some(key => data[key] !== undefined)) return null;
  return matrix(keys.map(row => row.map(key => data[key])));
}
function linearToSrgb(value) { const c = clamp(value); return c <= 0.0031308 ? c * 12.92 : 1.055 * c ** (1 / 2.4) - 0.055; }
function iniColor(data, id) {
  const values = ["x", "y", "z"].map(axis => data[axis + id]);
  return values.every(n => typeof n === "number" && Number.isFinite(n) && n >= 0 && n <= 8) ? hex(values.map(n => linearToSrgb(n) * 255)) : null;
}
function parseEdhmColors(startupRaw, advancedRaw, coefficients = IDENTITY) {
  const startup = parseIni(startupRaw), advanced = parseIni(advancedRaw), colors = matrixColors(coefficients), imported = [];
  const take = (role, condition, id) => {
    const value = condition ? iniColor(advanced, id) : null;
    if (value) { colors[role] = value; imported.push(role); return true; }
    return false;
  };
  if (!take("accent", startup.w206 === 100, 208)) take("accent", startup.x137 === 100, 232);
  take("text", startup.w75 === 100, 77);
  if (startup.y78 === 100) {
    const secondary = iniColor(advanced, 82);
    if (secondary) { colors.muted = mix(secondary, colors.bg, 0.26); imported.push("muted"); }
  }
  take("warn", startup.y104 === 100, 172);
  take("good", startup.y139 === 100, 246);
  take("danger", startup.y139 === 100, 247);
  // Backdrops have no one-to-one counterpart in the holographic game HUD.
  colors.bg = mix("#07090c", colors.accent, 0.055);
  colors.panel = mix("#0a0d12", colors.accent, 0.085);
  return { colors, imported };
}
async function readFile(filename, optional = false) {
  try {
    const info = await fsp.stat(filename);
    if (!info.isFile() || info.size > MAX_FILE_BYTES) throw new ThemeError("Файл палитры недоступен или превышает 1 МБ.", "FILE_TOO_LARGE");
    return { filename, raw: (await fsp.readFile(filename, "utf8")).replace(/^\uFEFF/, ""), modifiedAt: info.mtime.toISOString() };
  } catch (problem) {
    if (optional && ["ENOENT", "ENOTDIR", "EACCES", "EPERM"].includes(problem.code)) return null;
    if (problem instanceof ThemeError) throw problem;
    throw new ThemeError("Не удалось прочитать файл палитры. Проверьте путь и доступ к нему.", "FILE_UNAVAILABLE");
  }
}
async function importEdhm(directory, now) {
  const profile = await readFile(path.join(directory, "XML-Profile.ini"), true);
  if (!profile) return null;
  const coefficients = parseEdhmMatrix(profile.raw);
  if (!coefficients) return null;
  const startup = await readFile(path.join(directory, "Startup-Profile.ini"), true);
  const advanced = await readFile(path.join(directory, "Advanced.ini"), true);
  const selected = await readFile(path.join(directory, "ThemeSettings.json"), true);
  let name = "Цвета EDHM", imported = [], colors = matrixColors(coefficients);
  if (selected) { try { const data = JSON.parse(selected.raw); name = safeText(data.credits?.theme, 180) || name; } catch {} }
  if (startup && advanced) ({ colors, imported } = parseEdhmColors(startup.raw, advanced.raw, coefficients));
  const files = [profile, startup, advanced].filter(Boolean);
  const source = { kind: "edhm", name, path: directory, files: files.map(file => file.filename),
    modifiedAt: files.map(file => file.modifiedAt).sort().at(-1), importedAt: new Date(now).toISOString(),
    notes: [imported.length ? "Основные цвета взяты из конфигурации EDHM; фон и приглушённые оттенки подобраны для читаемости приложения." :
      "Прочитана матрица EDHM. Индивидуальные цвета элементов HUD не найдены; палитра рассчитана по матрице.",
      "Это снимок настроек. После смены темы игры нажмите «Скопировать цвета из игры» ещё раз."] };
  const settings = normalizeTheme({ mode: "game", preset: "blue", colors, matrix: coefficients, source });
  return { settings, palette: derivePalette(settings), source: settings.source };
}
async function importXml(filename, now, optional = true) {
  const file = await readFile(filename, optional);
  if (!file) return null;
  const coefficients = parseXmlMatrix(file.raw);
  if (!coefficients) return null;
  const source = { kind: "xml", name: "Матрица HUD Elite Dangerous", path: filename, files: [filename],
    modifiedAt: file.modifiedAt, importedAt: new Date(now).toISOString(), notes: [
      "Основной цвет рассчитан по матрице HUD. Фон и текст адаптированы для читаемости приложения.",
      "Матрица не содержит индивидуальных настроек EDHM. При установленном EDHM импортируйте его XML-Profile.ini."] };
  const settings = normalizeTheme({ mode: "game", preset: "blue", colors: matrixColors(coefficients), matrix: coefficients, source });
  return { settings, palette: derivePalette(settings), source: settings.source };
}
async function gameDirectories(homeDir) {
  const programFiles = [process.env["ProgramFiles(x86)"], process.env.ProgramFiles, "C:/Program Files (x86)"].filter(Boolean);
  const steamRoots = [...new Set(programFiles.map(dir => path.join(dir, "Steam")))];
  const libraries = [...steamRoots];
  for (const dir of steamRoots) {
    const file = await readFile(path.join(dir, "steamapps/libraryfolders.vdf"), true);
    if (!file) continue;
    for (const match of file.raw.matchAll(/"path"\s+"([^"\r\n]+)"/g)) libraries.push(match[1].replace(/\\\\/g, "\\"));
  }
  const results = libraries.flatMap(dir => ["elite-dangerous-odyssey-64", "elite-dangerous-64"].map(product =>
    path.join(dir, "steamapps/common/Elite Dangerous/Products", product)));
  const frontierRoots = [path.join(homeDir, "AppData/Local/Frontier_Developments/Products"),
    "C:/Program Files (x86)/Frontier/EDLaunch/Products", "C:/Program Files/Frontier/EDLaunch/Products"];
  for (const dir of frontierRoots) for (const product of ["elite-dangerous-odyssey-64", "elite-dangerous-64"]) results.push(path.join(dir, product));
  return [...new Set(results)];
}
async function readGamePalette(options = {}) {
  const now = typeof options.now === "function" ? options.now() : options.now ?? Date.now();
  if (!Number.isFinite(Number(now))) throw new ThemeError("Некорректное время импорта.");
  const homeDir = options.homeDir || os.homedir();
  if (options.filePath) {
    const filename = path.resolve(String(options.filePath));
    const base = path.basename(filename).toLowerCase();
    if (["xml-profile.ini", "advanced.ini", "startup-profile.ini", "themesettings.json"].includes(base)) {
      const result = await importEdhm(path.dirname(filename), now);
      if (result) return result;
    } else if (["graphicsconfigurationoverride.xml", "graphicsconfiguration.xml"].includes(base)) {
      const result = await importXml(filename, now, false);
      if (result) return result;
    } else throw new ThemeError("Выберите GraphicsConfigurationOverride.xml или XML-Profile.ini из папки EDHM-ini.", "FILE_TYPE_UNSUPPORTED");
    throw new ThemeError("В выбранном файле не найдена полная цветовая матрица HUD.", "MATRIX_NOT_FOUND");
  }
  const gameDirs = options.gameDir ? [path.resolve(String(options.gameDir))] : await gameDirectories(homeDir);
  for (const dir of gameDirs) {
    const result = await importEdhm(path.join(dir, "EDHM-ini"), now);
    if (result) return result;
  }
  // An unlinked EDHM UI cache is deliberately not imported: it may belong to
  // a disabled mod or an unapplied theme. Non-default installations can select
  // their actual XML-Profile.ini explicitly.
  const localRoots = [path.join(homeDir, "AppData/Local"), process.env.LOCALAPPDATA].filter(Boolean);
  for (const local of [...new Set(localRoots)]) {
    const result = await importXml(path.join(local, "Frontier Developments/Elite Dangerous/Options/Graphics/GraphicsConfigurationOverride.xml"), now);
    if (result) return result;
  }
  for (const dir of gameDirs) {
    const result = await importXml(path.join(dir, "GraphicsConfiguration.xml"), now);
    if (result) return result;
  }
  throw new ThemeError("Цветовая матрица игры не найдена. Укажите GraphicsConfigurationOverride.xml или XML-Profile.ini вручную; для EDHM нужна папка активной установки игры.", "GAME_PALETTE_NOT_FOUND");
}

module.exports = { ThemeError, PRESETS, COLOR_KEYS, IDENTITY, normalizeTheme, derivePalette, readGamePalette,
  parseXmlMatrix, parseEdhmMatrix, parseEdhmColors, transform, linearToSrgb };
