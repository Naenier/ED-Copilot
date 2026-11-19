import { extend } from './catalog.mjs';
import { execFileSync } from 'node:child_process';
import { existsSync, cpSync, mkdtempSync, mkdirSync, readFileSync, writeFileSync, rmSync } from 'node:fs';
import { createRequire } from 'node:module';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const require = createRequire(import.meta.url);
const repo = resolve(here, '../..');
const sourceRevision = '68c042ca6e3db62372cbbb2077cf972345511712';
const dataRevision = '0db9234b5b9ce8c939ea84133d7ce336eea88e27';
const almanacRevision = '42e874ed8f8b2ad89f99b9a21cc575851415f646';
const staging = mkdtempSync(join(tmpdir(), 'ed-copilot-fit-bundle-'));

function checkout(project, revision, existing, owner = 'EDCD') {
  if (!existing) {
    existing = join(staging, project);
    mkdirSync(existing);
    execFileSync('git', ['init', '-q', existing]);
    execFileSync('git', ['-C', existing, 'fetch', '-q', '--depth', '1', 'https://github.com/' + owner + '/' + project + '.git', revision]);
    execFileSync('git', ['-C', existing, 'checkout', '-q', 'FETCH_HEAD']);
  }
  if (execFileSync('git', ['-C', existing, 'rev-parse', 'HEAD'], { encoding: 'utf8' }).trim() !== revision) {
    throw new Error('Unexpected revision for ' + project);
  }
  return existing;
}

function patch(path, before, after) {
  const original = readFileSync(path, 'utf8');
  if (!original.includes(before)) throw new Error('Missing expected source in ' + path);
  writeFileSync(path, original.replace(before, after));
}

// Describe the numerical/factual catalogue without upstream explanatory prose or image assets.
function omitDescriptions(value) {
  if (Array.isArray(value)) return value.map(omitDescriptions);
  if (!value || typeof value !== 'object') return value;
  return Object.fromEntries(Object.entries(value).filter(([key]) => !['description', 'info', 'ukDiscript'].includes(key)).map(([key, data]) => [key, omitDescriptions(data)]));
}

try {
  const source = checkout('coriolis', sourceRevision, process.argv[2]);
  const data = checkout('coriolis-data', dataRevision, process.argv[3]);
  const upstream = join(staging, 'upstream');
  cpSync(join(source, 'src/app/shipyard'), join(upstream, 'shipyard'), { recursive: true });
  mkdirSync(join(upstream, 'utils'), { recursive: true });
  for (const file of ['UtilityFunctions', 'BlueprintFunctions', 'UrlGenerators', 'CompanionApiUtils', 'JournalUtils']) {
    cpSync(join(source, 'src/app/utils', file + '.js'), join(upstream, 'utils', file + '.js'));
  }
  cpSync(join(here, 'adapter.js'), join(staging, 'adapter.js'));

  const rawData = { Ships: require(join(data, 'ships')), Modules: require(join(data, 'modules')), Modifications: require(join(data, 'modifications')) };
  const almanac = checkout('Almanac-Core', almanacRevision, process.argv[4], 'Elite-Dangerous-Almanac');
  extend(rawData, almanac);
  const dataModule = join(staging, 'catalog.cjs');
  writeFileSync(dataModule, 'module.exports = ' + JSON.stringify(omitDescriptions(rawData)) + ';\n');

  patch(join(upstream, 'shipyard/Ship.js'), "const zlib = require('zlib');", "const zlib = require('zlib');\nconst { Buffer } = require('buffer');");
  patch(join(upstream, 'shipyard/Ship.js'), "new Buffer(Utils.fromUrlSafe(modstr), 'base64')", "Buffer.from(Utils.fromUrlSafe(modstr), 'base64')");
  // A load exactly at the reactor's capacity is still powered.
  const shipSource = join(upstream, 'shipyard/Ship.js');
  patch(shipSource, 'deployedSum >= this.powerAvailable', 'deployedSum - this.powerAvailable > 1e-9');
  patch(shipSource, 'retractedSum >= this.powerAvailable', 'retractedSum - this.powerAvailable > 1e-9');
  patch(shipSource, 'this.powerAvailable = this.standard[0].m.getPowerGeneration();', 'this.powerAvailable = this.standard[0].enabled ? this.standard[0].m.getPowerGeneration() : 0;');
  patch(shipSource, 'this.dryMass + (cargo || 0) + (fuel || 0) < this.standard[1].m.getMaxMass()', 'this.dryMass + (cargo || 0) + (fuel || 0) + (this.reserveFuelCapacity || 0) <= this.standard[1].m.getMaxMass()');
  // An invalid FSD imported from an external build must never make the fuel loop stall.
  // The upstream webpack build preserves namespace call receivers. Direct calls make
  // the same calculation functions portable to this tree-shaken browser bundle.
  const calculations = join(upstream, 'shipyard/Calculations.js');
  const receivers = new Set();
  const sourceCalculations = readFileSync(calculations, 'utf8').replace(/\bthis\.([A-Za-z_]+)\(/g, (_, name) => {
    receivers.add(name);
    return 'call_' + name + '(';
  });
  writeFileSync(calculations, sourceCalculations + '\n' + [...receivers].map(name => 'const call_' + name + ' = ' + name + ';').join('\n'));
  patch(join(upstream, 'shipyard/Calculations.js'), 'let fuelRemaining = fuel;', 'if (!(fsdMaxFuelPerJump > 0)) return 0;\n  let fuelRemaining = fuel;');
  patch(join(upstream, 'shipyard/Calculations.js'), 'while (fuelRemaining > 0) {', 'for (let i = 0; fuelRemaining > 0 && i < 10000; i++) {');
  patch(join(upstream, 'shipyard/Calculations.js'), 'return totalRange;', 'return fuelRemaining > 0 ? NaN : totalRange;');
  patch(join(upstream, 'utils/JournalUtils.js'), 'function _addModifications(', 'export function _addModifications(');
  patch(join(upstream, 'utils/JournalUtils.js'), 'value = (modifiers[i].Value / module.range - 1) * 100;', 'value = modifiers[i].OriginalValue === 0 ? 0 : (modifiers[i].Value / modifiers[i].OriginalValue - 1) * 10000;');
  // The adapter uses only the pure calculation portion of these modules.
  const react = join(staging, 'unused-ui.cjs');
  writeFileSync(react, 'module.exports = {};\n');

  const notices = [
    readFileSync(join(source, 'LICENSE.md'), 'utf8'),
    'Coriolis shipyard (https://github.com/EDCD/coriolis) revision ' + sourceRevision + '.',
    'Coriolis numerical catalogue (https://github.com/EDCD/coriolis-data) revision ' + dataRevision + '.',
    'Updated game measurements and module facts: Elite Dangerous Almanac (https://github.com/Elite-Dangerous-Almanac/Almanac-Core) revision ' + almanacRevision + '. See its ATTRIBUTIONS.md. EDSY by taleden (https://github.com/taleden/EDSY) is a source for some numerical facts and provides them under CC BY-NC 4.0 (https://creativecommons.org/licenses/by-nc/4.0/).',
  ];
  for (const dependency of ['lodash', 'lz-string', 'buffer', 'base64-js', 'ieee754', 'pako']) {
    const directory = join(here, 'node_modules', dependency);
    const license = ['LICENSE', 'LICENSE.md', 'LICENSE.txt', 'LICENCE'].find(file => existsSync(join(directory, file)));
    if (!license) throw new Error('Missing license for ' + dependency);
    notices.push(dependency + '\n\n' + readFileSync(join(directory, license), 'utf8'));
  }
  writeFileSync(join(repo, 'internal/frontend/ship-fit-engine.NOTICE.txt'), notices.join('\n\n'));

  const licenseHeader = '/*!\n' + notices.join('\n\n').replaceAll('*/', '* /') + '\n*/';
  const esbuild = require('esbuild');
  await esbuild.build({
    absWorkingDir: here,
    entryPoints: [join(staging, 'adapter.js')],
    outfile: join(repo, 'internal/frontend/ship-fit-engine.js'),
    platform: 'browser',
    format: 'iife',
    target: 'es2022',
    bundle: true,
    minify: true,
    drop: ['console'],
    loader: { '.js': 'jsx' },
    nodePaths: [join(here, 'node_modules')],
    alias: { 'coriolis-data/dist': dataModule, 'zlib': join(here, 'zlib.cjs'), 'react': react },
    legalComments: 'inline',
    banner: { js: '/*! Generated by scripts/ship-fit/build.mjs. Coriolis simulation: MIT, © Coriolis.io and Colin McLeod. Elite Dangerous game data © Frontier Developments plc. See ship-fit-engine.NOTICE.txt. */\n' + licenseHeader },
  });
} finally {
  rmSync(staging, { force: true, recursive: true });
}
