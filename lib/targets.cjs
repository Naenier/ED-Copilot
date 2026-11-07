"use strict";

const catalog = require("./catalog.cjs");

const TIER = { unknown: 0, low: 1, medium: 2, high: 3 };
const RING_NAMES = {
  metallic: "Металлическое",
  metalrich: "Богатое металлами",
  rocky: "Каменистое",
  icy: "Ледяное",
};
const RESERVES = {
  pristine: "Нетронутые",
  major: "Богатые",
  common: "Обычные",
  low: "Бедные",
  depleted: "Истощённые",
};

function keyOf(value) {
  return String(value ?? "").replace(/^\$/, "").replace(/_name;?$/i, "").replace(/[^a-z0-9]/gi, "").toLowerCase();
}

function ringKey(value) {
  const key = keyOf(value).replace(/^eringclass/, "");
  if (key.includes("metalrich")) return "metalrich";
  if (key.includes("metallic") || key.includes("metalic")) return "metallic";
  if (key.includes("rocky")) return "rocky";
  if (key.includes("icy") || key.includes("ice")) return "icy";
  return "";
}

function reserveKey(value) {
  return keyOf(value).replace(/resources$/, "");
}

function distance(from, to) {
  if (!Array.isArray(from) || !Array.isArray(to) || from.length !== 3 || to.length !== 3) return null;
  return Math.hypot(...from.map((number, index) => number - to[index]));
}

function tierMax(resources) {
  return resources.reduce((tier, resource) => TIER[resource.tier] > TIER[tier] ? resource.tier : tier, "unknown");
}

function resourceView(resource) {
  return { key: resource.key, name: resource.name, tier: resource.tier, methods: resource.methods ?? [],
    requiresCore: resource.requiresCore ?? false };
}

function buildTargets(world, signalMap, resources, commander, settings = {}) {
  const result = [];
  const commodities = resources.filter((resource) => resource.kind === "commodity");
  const lookup = new Map(resources.map((resource) => [resource.key, resource]));
  const selectedMethod = settings.method ?? "all";
  const findResource = (value) => lookup.get(catalog.getResource(value)?.key ?? keyOf(value));
  function ringResource(resource, type) {
    const available = resource.availability?.filter((entry) => (entry.ringTypes ?? []).map(ringKey).includes(type));
    const methods = available ? [...new Set(available.map((entry) => entry.method))] : resource.methods ?? [];
    const coreAccess = methods.includes("core") && methods.includes("surface") && !methods.includes("laser");
    return { ...resource, methods, requiresCore: coreAccess };
  }
  for (const system of world.values()) {
    for (const body of system.bodies.values()) {
      const base = {
        system: system.name, systemAddress: system.address, body: body.name, bodyId: body.id,
        distanceLy: distance(commander.coordinates, system.coordinates),
        distanceLs: body.distanceLs ?? null, gravity: body.gravity ?? null,
        landable: body.landable ?? false, source: body.source ?? "journal",
        updatedAt: body.timestamp ?? system.timestamp, coordinates: system.coordinates,
      };
      for (const ring of body.rings ?? []) {
        if (["surface-planet", "raw"].includes(selectedMethod)) continue;
        if (/belt/i.test(ring.name)) continue;
        const type = ringKey(ring.type);
        const reserve = reserveKey(body.reserve);
        const signals = signalMap.get(system.key + ":" + ring.name) ?? ring.signals ?? [];
        const hotspots = signals.map((signal) => {
          const symbol = signal.key ?? signal.Type ?? signal.name;
          const resource = findResource(symbol);
          const count = Number(signal.Count ?? signal.count) || 1;
          if (!resource) return { key: keyOf(symbol), name: signal.Type_Localised ?? signal.name ?? String(symbol),
            tier: "unknown", methods: [], count };
          if (resource.kind !== "commodity") return null;
          return { ...resourceView(ringResource(resource, type)), count };
        }).filter(Boolean);
        const possible = commodities.filter((resource) => {
          if (Array.isArray(resource.availability)) {
            return resource.availability.some((entry) => (entry.ringTypes ?? []).map(ringKey).includes(type));
          }
          return (resource.ringTypes ?? []).map(ringKey).includes(type);
        });
        const candidates = hotspots.length ? hotspots : possible.filter((resource) => TIER[resource.tier] >= TIER.medium);
        const useful = candidates.map((resource) => ringResource(findResource(resource.key) ?? resource, type))
          .filter((resource) => selectedMethod === "all" || resource.methods.includes(selectedMethod));
        if (selectedMethod !== "all" && !useful.length) continue;
        let tier = tierMax(useful);
        const reasons = [];
        let score = tier === "high" ? 70 : tier === "medium" ? 45 : 15;
        if (hotspots.length) {
          reasons.push("Горячие точки подтверждены сканированием: " + hotspots.map((entry) => entry.name).join(", "));
          score += 15;
          if (hotspots.some((resource) => resource.tier === "unknown")) {
            reasons.push("Часть сигналов отсутствует в справочнике. Их способ добычи и ценность пока неизвестны.");
          }
        } else {
          reasons.push("Оценка по типу кольца. Для подтверждения ресурсов нужны зонды ПСП и разведчик.");
          if (type === "icy") {
            tier = "low";
            score = 22;
          } else if (tier === "high") {
            tier = "medium";
            score = 50;
          }
        }
        const methods = [...new Set(useful.flatMap((resource) => resource.methods))];
        const laserRelevant = methods.includes("laser") && ["all", "laser"].includes(selectedMethod);
        if (reserve === "pristine" && laserRelevant) {
          score += 10;
          reasons.push("Нетронутые запасы — преимущество для лазерной добычи.");
        } else if (laserRelevant && (reserve === "depleted" || reserve === "low")) {
          score -= 18;
          reasons.push("Для лазерной добычи запасы слабые. Ядра оценивайте отдельно.");
        }
        if (type === "metalrich" && candidates.some((resource) => ["platinum", "painite"].includes(resource.key))) {
          reasons.push("Платина и пейнит здесь требуют разработки ядра. Для лазерной добычи этих ресурсов ищите металлическое кольцо.");
        }
        if (selectedMethod === "surface" && useful.some((resource) => resource.requiresCore)) {
          reasons.push("Для части ресурсов сначала нужно раскрыть ядро сейсмическими зарядами, затем снять отложения абразивным бластером.");
        }
        if (body.distanceLs > 10000) {
          score -= 8;
          reasons.push("Долгий перелёт от звезды прибытия.");
        }
        result.push({
          ...base, id: system.key + ":" + body.id + ":ring:" + ring.name, kind: "ring", ring: ring.name,
          ringType: RING_NAMES[type] ?? "Тип пока неизвестен", ringTypeKey: type,
          reserve: RESERVES[reserve] ?? "Запасы неизвестны", reserveKey: reserve,
          tier, score: Math.max(0, Math.min(100, score)), confidence: hotspots.length ? "confirmed" : "potential",
          hotspots, materials: [], resources: useful.map(resourceView), methods, reasons,
          recommendation: hotspots.length ? "Выберите метод добычи и проверьте астероид разведчиком." : "Разведать кольцо перед добычей.",
          miningSites: 0, compositionUnknown: false,
        });
      }

      const materials = (body.materials ?? []).map((material) => {
        const resource = findResource(material.key ?? material.Name);
        if (!resource || resource.kind !== "raw") return null;
        return { key: resource.key, name: resource.name, percent: material.percent ?? material.Percent ?? 0,
          grade: resource.grade ?? 1, tier: resource.tier };
      }).filter(Boolean).sort((a, b) => b.grade - a.grade || b.percent - a.percent);
      const valuable = materials.filter((entry) => entry.grade >= 3);
      const signals = signalMap.get(system.key + ":" + body.name) ?? [];
      const miningSites = signals.filter((signal) => /PlanetaryMiningLocation/i.test(signal.Type ?? signal.key ?? signal.name))
        .reduce((sum, signal) => sum + (Number(signal.Count ?? signal.count) || 0), 0);
      const geology = signals.filter((signal) => /Geological|Geology/i.test(signal.Type ?? signal.key ?? signal.name))
        .reduce((sum, signal) => sum + (Number(signal.Count ?? signal.count) || 0), 0);
      if (!miningSites && !(body.landable && valuable.length)) continue;
      if (!["all", "raw", "surface-planet"].includes(selectedMethod)) continue;
      if (selectedMethod === "surface-planet" && !miningSites) continue;
      if (selectedMethod === "raw" && !(body.landable && valuable.length)) continue;
      const showRaw = selectedMethod !== "surface-planet";
      const reasons = [];
      const siteFinds = body.surfaceFinds ?? [];
      const observed = new Map();
      for (const site of siteFinds) {
        for (const entry of site.resources ?? []) {
          const previous = observed.get(entry.key);
          if (previous) previous.count += entry.count;
          else observed.set(entry.key, { ...entry });
        }
        if (site.resources?.length) {
          reasons.push(site.site + ". Переработка по журналу: " + site.resources.map((entry) => entry.name + " " + entry.count + " т").join(", ") + ".");
        }
      }
      const observedResources = [...observed.values()];
      if (miningSites) reasons.push("Сканер подтвердил участки планетарной добычи: " + miningSites + ". Состав залежей требует разведки.");
      if (showRaw && valuable.length) reasons.push("Сырьё высоких категорий: " + valuable.map((entry) => entry.name + " " + entry.percent + "%").join(", "));
      if (geology) reasons.push("Есть геологические сигналы: " + geology + ".");
      if (body.gravity > 1.5) reasons.push("Высокая гравитация усложняет посадку.");
      const observedTier = tierMax(observedResources.map((entry) => findResource(entry.key)).filter(Boolean));
      const tier = showRaw && valuable.some((entry) => entry.grade === 4) ? "high"
        : (showRaw && valuable.length) || observedTier === "medium" || observedTier === "high" ? "medium" : "unknown";
      const score = Math.min(100, (tier === "high" ? 70 : tier === "medium" ? 48 : 35)
        + (miningSites ? 10 : 0) + (geology ? 8 : 0) - (body.gravity > 1.5 ? 10 : 0));
      result.push({
        ...base, id: system.key + ":" + body.id + ":surface", kind: "surface", ring: "", ringType: "", reserve: "",
        tier, score, confidence: miningSites || materials.length ? "confirmed" : "potential",
        hotspots: [], materials, resources: showRaw ? valuable.map((entry) => resourceView(findResource(entry.key))) : [],
        methods: [...(miningSites && selectedMethod !== "raw" ? ["surface-planet"] : []), ...(showRaw && valuable.length ? ["raw"] : [])],
        miningSites, compositionUnknown: miningSites > 0 && selectedMethod !== "raw", geology,
        observedResources, siteFinds,
        reasons, recommendation: miningSites && selectedMethod !== "raw" ? "Выберите участок в навигации и разведайте залежь на Rhino."
          : "Посадка ради сырья для инженеров и синтеза. Проценты не определяют скорость сбора.",
      });
    }
  }
  return result.sort((a, b) => b.score - a.score || (a.distanceLy ?? Infinity) - (b.distanceLy ?? Infinity));
}

module.exports = { keyOf, ringKey, reserveKey, buildTargets, distance, TIER, RING_NAMES, RESERVES };
