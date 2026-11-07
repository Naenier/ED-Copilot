"use strict";
// ED Copilot's read-only Windows focus monitor. It sends no window titles or input.
const { spawn } = require("node:child_process");
const path = require("node:path");
const readline = require("node:readline");

function processName(value) {
  if (typeof value !== "string") return null;
  const name = value.split(/[\\/]/).at(-1).replace(/\.exe$/i, "").trim().toLowerCase();
  return name.length <= 180 && !/[\r\n\u0000]/.test(name) ? name : null;
}
function point(value) {
  return value && typeof value.x === "number" && typeof value.y === "number" &&
    Number.isFinite(value.x) && Number.isFinite(value.y) && Math.abs(value.x) < 10_000_000 && Math.abs(value.y) < 10_000_000
    ? { x: Math.round(value.x), y: Math.round(value.y) } : null;
}
function parseForegroundLine(line) {
  let record;
  try { record = JSON.parse(line); } catch {
    const [name, location] = String(line).split("\t");
    const pair = /^(-?\d+),(-?\d+)$/.exec(location || "");
    return { name: processName(name), at: pair ? point({ x: Number(pair[1]), y: Number(pair[2]) }) : null };
  }
  return { name: processName(record?.name), at: point(record?.at) };
}
function isGame(name) {
  return /^(?:elitedangerous(?:32|64)?|elitedangerousodyssey(?:32|64)?)$/.test(processName(name) || "");
}
function isGameOrOwn(name, own = []) {
  const current = processName(name);
  return current !== null && (isGame(current) || Array.from(own || []).some(candidate => processName(candidate) === current));
}
function nativeWatcherPath() { return null; }

function monitorScript(supervisorId) {
  if (!Number.isSafeInteger(supervisorId) || supervisorId <= 0) throw new Error("Invalid monitor supervisor");
  return String.raw`
$ErrorActionPreference = 'Stop'
$supervisorId = ${supervisorId}
Add-Type -TypeDefinition @"
using System;
using System.Runtime.InteropServices;
public static class CopilotWindowProbe {
  [StructLayout(LayoutKind.Sequential)]
  public struct Box { public int X1; public int Y1; public int X2; public int Y2; }
  [DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr window, out uint processId);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr window, out Box box);
  [DllImport("user32.dll")] public static extern IntPtr SetThreadDpiAwarenessContext(IntPtr context);
}
"@
try { [void][CopilotWindowProbe]::SetThreadDpiAwarenessContext([IntPtr](-4)) } catch {}
$previous = $null
while ($true) {
  try { $supervisor = [System.Diagnostics.Process]::GetProcessById($supervisorId); $supervisor.Dispose() } catch { break }
  $handle = [CopilotWindowProbe]::GetForegroundWindow()
  [uint32]$ownerId = 0
  [void][CopilotWindowProbe]::GetWindowThreadProcessId($handle, [ref]$ownerId)
  $processLabel = ''
  if ($ownerId -gt 0) {
    try {
      $owner = [System.Diagnostics.Process]::GetProcessById($ownerId)
      $processLabel = $owner.ProcessName
      $owner.Dispose()
    } catch {}
  }
  $box = New-Object CopilotWindowProbe+Box
  $center = $null
  if ([CopilotWindowProbe]::GetWindowRect($handle, [ref]$box)) {
    $center = @{ x = [int](($box.X1 + $box.X2) / 2); y = [int](($box.Y1 + $box.Y2) / 2) }
  }
  $record = @{ name = $processLabel; at = $center } | ConvertTo-Json -Compress -Depth 3
  if ($record -ne $previous) {
    try { [Console]::Out.WriteLine($record); [Console]::Out.Flush() } catch { break }
    $previous = $record
  }
  Start-Sleep -Milliseconds 500
}
`;
}

function watchForeground(onName, unusedExecutable = null, dependencies = {}) {
  const launch = dependencies.spawn || spawn;
  const platform = dependencies.platform || process.platform;
  let stopped = false, child = null, lines = null, restart = null, attempts = 0;
  const notify = (name, at) => { if (!stopped) try { onName(name, at); } catch {} };
  if (platform !== "win32") {
    notify(null, null);
    return { stop() { stopped = true; } };
  }
  function start() {
    if (stopped) return;
    let exited = false;
    const failed = () => {
      if (exited || stopped) return;
      exited = true;
      lines?.close(); lines = null;
      child?.kill(); child = null;
      notify(null, null);
      if (++attempts <= 3) {
        restart = setTimeout(start, 1000 * 2 ** (attempts - 1));
        restart.unref?.();
      }
    };
    try {
      const executable = dependencies.powershell || path.join(process.env.SystemRoot || "C:\\Windows", "System32", "WindowsPowerShell", "v1.0", "powershell.exe");
      child = launch(executable, ["-NoLogo", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-EncodedCommand", Buffer.from(monitorScript(process.pid), "utf16le").toString("base64")],
        { windowsHide: true, stdio: ["ignore", "pipe", "ignore"] });
      lines = readline.createInterface({ input: child.stdout, crlfDelay: Infinity });
      lines.on("line", line => {
        if (line.length > 2048) return;
        const parsed = parseForegroundLine(line);
        if (parsed.name !== null) notify(parsed.name, parsed.at);
      });
      child.once("error", failed); child.once("exit", failed);
    } catch { failed(); }
  }
  start();
  return { stop() { stopped = true; clearTimeout(restart); lines?.close(); child?.kill(); lines = null; child = null; } };
}

function watchForegroundOffMain(utilityProcess, hostPath, onName, unusedExecutable = null) {
  let stopped = false, worker = null, direct = null, watchdog = null;
  const notify = (name, at) => { if (!stopped) try { onName(name, at); } catch {} };
  const fallback = () => {
    if (stopped || direct) return;
    clearTimeout(watchdog);
    worker?.kill(); worker = null;
    direct = watchForeground(notify);
  };
  if (!utilityProcess?.fork || !hostPath || process.platform !== "win32") return watchForeground(notify);
  try {
    worker = utilityProcess.fork(hostPath, [], { serviceName: "ED Copilot focus monitor", stdio: "ignore" });
    worker.on("message", record => {
      if (record?.type !== "foreground") return;
      clearTimeout(watchdog);
      const parsed = { name: record.name === null ? null : processName(record.name), at: point(record.at) };
      notify(parsed.name, parsed.at);
    });
    worker.once("exit", fallback);
    worker.once("error", fallback);
    watchdog = setTimeout(fallback, 15000);
    watchdog.unref?.();
  } catch { fallback(); }
  return { stop() { stopped = true; clearTimeout(watchdog); direct?.stop(); try { worker?.postMessage({ type: "stop" }); } catch {} worker?.kill(); worker = null; } };
}

module.exports = { watchForeground, watchForegroundOffMain, isGameOrOwn, isGame, parseForegroundLine, nativeWatcherPath, monitorScript };

