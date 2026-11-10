// Install the build-only tools into a temporary directory with:
// npm install --prefix <directory> --ignore-scripts sharp@0.34.5 @resvg/resvg-js@2.6.2
// Set SHIP_ATLAS_TOOLS to that directory, then run node scripts/build-ship-atlas.mjs.
// Pass ship keys as arguments to regenerate a subset.
import { mkdir, readFile, writeFile } from 'node:fs/promises'
import { createRequire } from 'node:module'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const require = process.env.SHIP_ATLAS_TOOLS
  ? createRequire(join(resolve(process.env.SHIP_ATLAS_TOOLS), 'package.json'))
  : createRequire(import.meta.url)
const sharp = require('sharp')
const { Resvg } = require('@resvg/resvg-js')

const root = fileURLToPath(new URL('../', import.meta.url))
const media = join(root, 'internal/reference/media')
const revision = '42e874ed8f8b2ad89f99b9a21cc575851415f646'
const upstream = `https://raw.githubusercontent.com/Elite-Dangerous-Almanac/Almanac-Core/${revision}/assets/ships`
const cache = join(tmpdir(), `ed-copilot-ship-atlas-${revision}`)
const width = 1440
const height = 884
const margin = 20
const gap = 16
const hero = { x: margin, y: margin, width: width - 2 * margin, height: 504 }
const panel = { x: margin, y: hero.y + hero.height + gap, width: (hero.width - gap) / 2, height: 324 }
const maxSize = 200_000

async function download(url, key) {
  const file = join(cache, key)
  try {
    return await readFile(file)
  } catch (error) {
    if (error.code !== 'ENOENT') throw error
  }
  const response = await fetch(url, { signal: AbortSignal.timeout(25_000) })
  if (!response.ok) throw new Error(`${response.status} ${url}`)
  const bytes = Buffer.from(await response.arrayBuffer())
  await writeFile(file, bytes)
  return bytes
}

async function shipPhoto(key, url) {
  if (url) {
    try {
      const photo = await download(url, `${key}-photo`)
      await sharp(photo).metadata()
      return photo
    } catch (error) {
      console.warn(`${key}: using bundled photograph (${error.message})`)
    }
  }
  return readFile(join(media, `ship-${key}.jpg`))
}

// Weight bright ship surfaces so the original camera composition can be kept
// while giving the hull room on both sides of the wider plate.
async function photoFocus(photo) {
  const { data, info } = await sharp(photo).resize({ width: 96 }).greyscale().raw()
    .toBuffer({ resolveWithObject: true })
  let total = 0
  let weightedX = 0
  for (let y = 0; y < info.height; y += 1) {
    for (let x = 0; x < info.width; x += 1) {
      const strength = Math.max(0, data[y * info.width + x] - 30) ** 2
      weightedX += strength * (x + 0.5)
      total += strength
    }
  }
  return total ? weightedX / total / info.width : 0.5
}

async function renderHero(photo) {
  const focus = await photoFocus(photo)
  const { data, info } = await sharp(photo).resize({ height: hero.height, width: hero.width, fit: 'inside' })
    .png().toBuffer({ resolveWithObject: true })
  const photoLeft = Math.max(0, Math.min(hero.width - info.width, Math.round(hero.width / 2 - focus * info.width)))
  const photoTop = Math.floor((hero.height - info.height) / 2)
  const edge = Math.min(34, info.width / 12)
  const mask = Buffer.from(`<svg xmlns="http://www.w3.org/2000/svg" width="${info.width}" height="${info.height}">
    <defs><linearGradient id="edge" x1="0" x2="1" gradientUnits="objectBoundingBox">
      <stop offset="0" stop-color="#fff" stop-opacity="0"/><stop offset="${edge / info.width}" stop-color="#fff"/>
      <stop offset="${1 - edge / info.width}" stop-color="#fff"/><stop offset="1" stop-color="#fff" stop-opacity="0"/>
    </linearGradient></defs><rect width="100%" height="100%" fill="url(#edge)"/>
  </svg>`)
  const foreground = await sharp(data).ensureAlpha().composite([{ input: mask, blend: 'dest-in' }]).png().toBuffer()
  const backdrop = await sharp(data).extend({
    left: photoLeft, right: hero.width - photoLeft - info.width,
    top: photoTop, bottom: hero.height - photoTop - info.height, extendWith: 'mirror',
  }).png().toBuffer()
  const toned = await sharp(backdrop).blur(28)
    .modulate({ brightness: 0.24, saturation: 0.58 }).png().toBuffer()
  const assembled = await sharp(toned).composite([{ input: foreground, left: photoLeft, top: photoTop }]).png().toBuffer()
  const corners = Buffer.from(`<svg xmlns="http://www.w3.org/2000/svg" width="${hero.width}" height="${hero.height}"><rect width="100%" height="100%" rx="10" fill="#fff"/></svg>`)
  return sharp(assembled).ensureAlpha().composite([{ input: corners, blend: 'dest-in' }]).png().toBuffer()
}

function plateFrame() {
  const sections = [panel, { ...panel, x: panel.x + panel.width + gap }].map(({ x, y, width, height }) => `
    <rect x="${x}" y="${y}" width="${width}" height="${height}" rx="10" fill="#101820"/>
    <rect x="${x + 1}" y="${y + 1}" width="${width - 2}" height="${height - 2}" rx="9" fill="url(#grid)"/>
    <path d="M${x + 24} ${y + 14}h-10v10m${width - 38} -10h10v10 M${x + 14} ${y + height - 24}v10h10m${width - 48} 0h10v-10" fill="none" stroke="#56738b" stroke-width="1"/>
    <rect x="${x + 0.5}" y="${y + 0.5}" width="${width - 1}" height="${height - 1}" rx="9" fill="none" stroke="#2a3b49"/>`).join('')
  return Buffer.from(`<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}">
    <defs><pattern id="grid" width="26" height="26" patternUnits="userSpaceOnUse"><path d="M0 26V0h26" fill="none" stroke="#2b3c4a" stroke-opacity=".42" stroke-width="1"/></pattern></defs>
    <rect width="100%" height="100%" fill="#0c1117"/>${sections}
  </svg>`)
}

async function prepareSchematic(svg) {
  const darkTheme = svg.toString('utf8')
    .replace(/(<svg\b[^>]*\scolor=")[^"]+/, '$1#ceddec')
    .replace(/fill-opacity="[\d.]+"/g, 'fill-opacity="0.26"')
  const raster = new Resvg(darkTheme, { fitTo: { mode: 'width', value: 1440 }, font: { loadSystemFonts: false } })
    .render().asPng()
  return sharp(raster).trim({ background: '#00000000', threshold: 8 }).png().toBuffer({ resolveWithObject: true })
}

async function renderSchematic(image, rotate, scale) {
  const { data, info } = image
  const dimensions = { width: Math.round(info.width * scale), height: Math.round(info.height * scale) }
  const resized = await sharp(data).resize(dimensions.width, dimensions.height).png().toBuffer()
  return rotate
    ? sharp(resized).rotate(90).png().toBuffer({ resolveWithObject: true })
    : { data: resized, info: dimensions }
}

async function buildPlate(ship, photoURL) {
  const { key, symbol } = ship
  const svgURL = `${upstream}/${encodeURIComponent(symbol)}`
  const [photo, top, bottom] = await Promise.all([
    shipPhoto(key, photoURL),
    download(`${svgURL}/schematic-top.svg`, `${key}-top.svg`).then(prepareSchematic),
    download(`${svgURL}/schematic-bottom.svg`, `${key}-bottom.svg`).then(prepareSchematic),
  ])
  const rotate = top.info.height > top.info.width
  const maxWidth = Math.max(top.info.width, bottom.info.width)
  const maxHeight = Math.max(top.info.height, bottom.info.height)
  const scale = Math.min((panel.width - 60) / (rotate ? maxHeight : maxWidth), (panel.height - 46) / (rotate ? maxWidth : maxHeight))
  const [heroRaster, ...projections] = await Promise.all([
    renderHero(photo),
    renderSchematic(top, rotate, scale),
    renderSchematic(bottom, rotate, scale),
  ])
  const layers = [
    { input: heroRaster, left: hero.x, top: hero.y },
    ...projections.map(({ data, info }, index) => ({
      input: data,
      left: panel.x + index * (panel.width + gap) + Math.floor((panel.width - info.width) / 2),
      top: panel.y + Math.floor((panel.height - info.height) / 2),
    })),
  ]
  const plate = await sharp(plateFrame()).composite(layers).png().toBuffer()
  for (const quality of [88, 82, 76, 70, 64]) {
    const output = await sharp(plate).jpeg({ quality, mozjpeg: true, chromaSubsampling: '4:4:4' }).toBuffer()
    if (output.length <= maxSize) {
      await writeFile(join(media, `ship-atlas-${key}.jpg`), output)
      console.log(`${key}: ${(output.length / 1000).toFixed(1)} KB`)
      return
    }
  }
  throw new Error(`${key}: plate exceeds the ${maxSize}-byte image budget`)
}

const catalogue = JSON.parse(await readFile(join(root, 'internal/data/encyclopedia-ships.json'), 'utf8'))
const metadata = JSON.parse(await readFile(join(media, 'metadata.json'), 'utf8'))
const originals = new Map(metadata.map(({ file, url }) => [file, url]))
const requested = new Set(process.argv.slice(2))
const ships = requested.size ? catalogue.filter(({ key }) => requested.has(key)) : catalogue
if (!ships.length || (requested.size && requested.size !== ships.length)) throw new Error('Unknown ship key')
await mkdir(cache, { recursive: true })
const batchSize = 4
for (let offset = 0; offset < ships.length; offset += batchSize) {
  await Promise.all(ships.slice(offset, offset + batchSize).map((ship) => buildPlate(ship, originals.get(`ship-${ship.key}.jpg`))))
}
