// Install the build-only tools into a temporary directory with:
// npm install --prefix <directory> --ignore-scripts sharp@0.34.5 @resvg/resvg-js@2.6.2
// Set SHIP_ATLAS_TOOLS to that directory, then run node scripts/build-ship-atlas.mjs.
// Pass ship keys as arguments to regenerate a subset.
import { mkdir, readFile, writeFile } from 'node:fs/promises'
import { createHash } from 'node:crypto'
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

async function shipPhoto(key, source) {
  const urlHash = createHash('sha256').update(source.url).digest('hex').slice(0, 16)
  return download(source.url, `${key}-${urlHash}-photo`)
}

function anchoredCrop(width, height, targetWidth, targetHeight, [x, y] = [0.5, 0.5]) {
  const cropWidth = Math.min(width, Math.round(height * targetWidth / targetHeight))
  const cropHeight = Math.min(height, Math.round(width * targetHeight / targetWidth))
  const left = Math.max(0, Math.min(width - cropWidth, Math.round(x * width - cropWidth / 2)))
  const top = Math.max(0, Math.min(height - cropHeight, Math.round(y * height - cropHeight / 2)))
  return { left, top, width: cropWidth, height: cropHeight }
}

async function framePhoto(photo, source, width, height) {
  const original = sharp(photo)
  const { width: sourceWidth, height: sourceHeight } = await original.metadata()
  const background = source.background || '#0c1117'
  if (source.crop) {
    const [x, y, w, h] = source.crop
    const left = Math.round(x * sourceWidth)
    const top = Math.round(y * sourceHeight)
    original.extract({ left, top, width: Math.round((x + w) * sourceWidth) - left, height: Math.round((y + h) * sourceHeight) - top })
  } else if (source.fit !== 'contain') {
    original.extract(anchoredCrop(sourceWidth, sourceHeight, width, height, source.focus))
  }
  if (source.fit === 'contain' && source.background) {
    // Studio images have an empty backdrop at the edges. Continue that color
    // into the panorama while preserving the original ship silhouette.
    const { data, info } = await original.resize({ width, height, fit: 'inside' })
      .flatten({ background }).png().toBuffer({ resolveWithObject: true })
    const left = Math.floor((width - info.width) / 2)
    const top = Math.floor((height - info.height) / 2)
    return sharp(data).extend({ left, top, right: width - info.width - left,
      bottom: height - info.height - top, extendWith: 'copy' })
  }
  return original.resize({ width, height, fit: source.fit || 'cover', background })
    .flatten({ background })
}

async function renderHero(photo, source) {
  const framed = await framePhoto(photo, source, hero.width, hero.height)
  const assembled = await framed.png().toBuffer()
  const corners = Buffer.from(`<svg xmlns="http://www.w3.org/2000/svg" width="${hero.width}" height="${hero.height}"><rect width="100%" height="100%" rx="10" fill="#fff"/></svg>`)
  return sharp(assembled).ensureAlpha().composite([{ input: corners, blend: 'dest-in' }]).png().toBuffer()
}

async function writeJPEG(raster, file, url, width, height, limit) {
  for (const quality of [86, 76, 66, 56]) {
    const output = await raster.clone().jpeg({ quality, mozjpeg: true }).toBuffer()
    if (output.length <= limit) {
      await writeFile(join(media, file), output)
      return { file, url, width, height, bytes: output.length,
        sha256: createHash('sha256').update(output).digest('hex') }
    }
  }
  throw new Error(`${file}: image exceeds the ${limit}-byte image budget`)
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

async function writeTopPreview(key, url, top, rotate) {
  const width = 320, height = 180
  const w = rotate ? top.info.height : top.info.width
  const h = rotate ? top.info.width : top.info.height
  const schematic = await renderSchematic(top, rotate, Math.min(282 / w, 144 / h))
  const grid = Buffer.from(`<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}">
    <defs><pattern id="grid" width="15" height="15" patternUnits="userSpaceOnUse"><path d="M0 15V0h15" fill="none" stroke="#263945" stroke-opacity=".45"/></pattern></defs>
    <rect width="100%" height="100%" fill="#101820"/><rect width="100%" height="100%" fill="url(#grid)"/>
  </svg>`)
  const raster = sharp(grid).composite([{ input: schematic.data,
    left: Math.floor((width - schematic.info.width) / 2),
    top: Math.floor((height - schematic.info.height) / 2) }])
  return writeJPEG(raster, `thumb-ship-${key}.jpg`, url, width, height, 8_000)
}

async function buildPlate(ship, source) {
  const { key, symbol } = ship
  const svgURL = `${upstream}/${encodeURIComponent(symbol)}`
  const [photo, top, bottom] = await Promise.all([
    shipPhoto(key, source),
    download(`${svgURL}/schematic-top.svg`, `${key}-top.svg`).then(prepareSchematic),
    download(`${svgURL}/schematic-bottom.svg`, `${key}-bottom.svg`).then(prepareSchematic),
  ])
  const rotate = top.info.height > top.info.width
  const maxWidth = Math.max(top.info.width, bottom.info.width)
  const maxHeight = Math.max(top.info.height, bottom.info.height)
  const scale = Math.min((panel.width - 60) / (rotate ? maxHeight : maxWidth), (panel.height - 46) / (rotate ? maxWidth : maxHeight))
  const [heroRaster, ...projections] = await Promise.all([
    renderHero(photo, source),
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
      const photoOnly = await framePhoto(photo, { ...source, crop: undefined }, 640, 360)
      return Promise.all([
        writeJPEG(photoOnly, `ship-${key}.jpg`, source.url, 640, 360, 150_000),
        writeTopPreview(key, `${svgURL}/schematic-top.svg`, top, rotate),
      ])
    }
  }
  throw new Error(`${key}: plate exceeds the ${maxSize}-byte image budget`)
}

const catalogue = JSON.parse(await readFile(join(root, 'internal/data/encyclopedia-ships.json'), 'utf8'))
const metadata = JSON.parse(await readFile(join(media, 'metadata.json'), 'utf8'))
const photos = JSON.parse(await readFile(join(root, 'docs/ship-photo-sources.json'), 'utf8'))
const requested = new Set(process.argv.slice(2))
const ships = requested.size ? catalogue.filter(({ key }) => requested.has(key)) : catalogue
if (!ships.length || (requested.size && requested.size !== ships.length)) throw new Error('Unknown ship key')
for (const { key } of ships) {
  if (!photos[key]?.url || !photos[key]?.source) throw new Error(`Missing curated photo: ${key}`)
}
await mkdir(cache, { recursive: true })
const batchSize = 4
const updated = new Map()
for (let offset = 0; offset < ships.length; offset += batchSize) {
  const rows = await Promise.all(ships.slice(offset, offset + batchSize).map(ship => buildPlate(ship, photos[ship.key])))
  for (const row of rows.flat()) updated.set(row.file, row)
}
const records = metadata.map(record => updated.get(record.file) || record)
await writeFile(join(media, 'metadata.json'), JSON.stringify(records, null, 2) + '\n')
