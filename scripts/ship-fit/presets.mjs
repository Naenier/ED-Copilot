import assert from "node:assert/strict";
import { readFileSync, writeFileSync } from "node:fs";
import { gunzipSync } from "node:zlib";
import "../../internal/frontend/ship-fit-engine.js";

const engine = globalThis.EDCopilotShipFitEngine;
const seeds = JSON.parse(readFileSync(new URL("./preset-sources.json", import.meta.url), "utf8"));

// Each hull has a primary activity. Secondary fits solve a different job or use a
// materially different operating setup on hulls where both variants make sense.
const assignments = {
    adder: ["trade", "exploration"],
    alliance_challenger: ["combat", "anti-xeno/enhanced"],
    alliance_chieftain: ["anti-xeno", "combat"],
    alliance_crusader: ["combat/fighter"],
    anaconda: ["exploration", "combat/large", "rescue"],
    anaconda_mk_ii: ["combat/large", "trade"],
    asp: ["exploration", "laser-mining"],
    asp_scout: ["exploration", "salvage"],
    beluga: ["passengers/luxury", "rescue"],
    explorer_nx: ["exploration/expedition"],
    cobra_mk_iii: ["trade", "combat/starter", "exploration"],
    cobra_mk_iv: ["salvage", "core-mining"],
    cobramkv: ["anti-xeno/shards", "exobiology", "laser-mining"],
    imperial_corsair: ["anti-xeno", "combat", "trade"],
    diamondback_explorer: ["exobiology", "exploration"],
    diamondback: ["exploration", "combat"],
    dolphin: ["passengers/luxury", "exploration", "rescue"],
    eagle: ["combat/starter"],
    federal_assault_ship: ["combat/armour"],
    federal_corvette: ["combat/large"],
    federal_dropship: ["laser-mining", "combat/armour"],
    federal_gunship: ["combat/fighter"],
    fer_de_lance: ["combat/large"],
    hauler: ["trade", "exploration"],
    imperial_clipper: ["core-mining", "trade"],
    imperial_courier: ["exobiology", "combat"],
    imperial_cutter: ["trade", "laser-mining"],
    imperial_eagle: ["racing"],
    keelback: ["laser-mining", "combat/fighter"],
    kestrel: ["combat"],
    krait_mkii: ["anti-xeno", "combat/fighter", "core-mining"],
    krait_phantom: ["exploration/expedition", "rescue"],
    lynx_highliner: ["passengers", "rescue"],
    mamba: ["combat/fragments"],
    mandalay: ["exploration/expedition", "exobiology"],
    orca: ["passengers/luxury", "exploration"],
    panthermkii: ["trade/guarded", "trade/ferry"],
    python: ["trade", "core-mining", "passengers"],
    python_nx: ["combat/fragments"],
    sidewinder: ["combat/starter", "trade"],
    type_10_defender: ["combat/fighter", "laser-mining"],
    type_11_prospector: ["laser-mining/volley", "core-mining"],
    type_6_transporter: ["trade"],
    type_7_transport: ["trade", "rescue"],
    type_8_transport: ["trade", "laser-mining"],
    type_9_heavy: ["trade", "laser-mining"],
    viper: ["combat/starter"],
    viper_mk_iv: ["combat/armour", "exobiology"],
    vulture: ["combat"],
};

const profiles = {
    combat: ["Наёмник PvE", "PvE bounty hunter",
        "Лазеры снимают щиты, мультипушки добивают корпус. Поставьте лазеры и кинетику в разные группы огня.",
        "Strip shields with lasers and finish the hull with multicannons. Keep lasers and kinetic weapons in separate fire groups."],
    "combat/starter": ["Бой PvE без инженеров", "Unengineered PvE fighter",
        "Стартовый комплект для охоты на пиратов в обычных зонах добычи. Подходите к цели вместе с охраной системы.",
        "Beginner outfit for pirate hunting in regular resource sites. Fight alongside system security."],
    "combat/armour": ["Броневой боец PvE", "Armoured PvE fighter",
        "Двухпоточный щит быстро восстанавливается, усиления прикрывают корпус и модули, пока он заряжается.",
        "The bi-weave recovers quickly, with hull and module reinforcement to cover the recharge."],
    "combat/large": ["Тяжёлый охотник PvE", "Heavy PvE bounty hunter",
        "В тяжёлых гнёздах стоят мультипушки с хорошим пробитием. Держите тепло под контролем при длительной стрельбе лазерами.",
        "High-penetration multicannons use the largest hardpoints. Watch heat during prolonged laser fire."],
    "combat/fighter": ["Боец с истребителем", "Fighter-supported PvE",
        "Наймите пилота NPC для истребителя. Его атаки отвлекают цель, пока вы ведёте огонь с основного корабля.",
        "Hire an NPC fighter pilot. The fighter draws the target's attention while your ship deals damage."],
    "combat/fragments": ["Ближний бой PvE", "Close-range PvE",
        "Фрагментные пушки рассчитаны на короткое окно огня вблизи цели. Не тратьте боекомплект на дальних разворотах.",
        "Fragment cannons deliver short bursts at close range. Save ammunition during the long turns."],
    "anti-xeno": ["Таргоиды PvE", "Anti-xeno PvE",
        "Улучшенные AX-мультипушки подходят для разведчиков и групповых боёв. Просканируйте перехватчика, чтобы навести пушки на раскрытое сердце.",
        "Enhanced AX multicannons suit Scouts and group fights. Scan the Interceptor to target its exerted heart with the gimballed weapons."],
    "anti-xeno/enhanced": ["Таргоиды PvE с AX-пушками", "Enhanced AX PvE",
        "Улучшенные AX-мультипушки подходят для разведчиков. Для самостоятельной охоты на перехватчиков выбирайте сборку с оружием Стражей.",
        "Enhanced AX multicannons suit Scouts. For solo Interceptor hunting, choose a Guardian weapon build."],
    "anti-xeno/shards": ["Таргоиды PvE с осколочниками", "Shard-cannon AX PvE",
        "Осколочные пушки эффективны вблизи. Для модифицированных осколочников нужен соответствующий обмен у технологического брокера.",
        "Shard cannons work at close range. Modified shards require the appropriate technology-broker exchange."],
    trade: ["Защищённый грузовоз", "Shielded freight",
        "Щит рассчитан на стыковку и выход из перехвата. Проверяйте дальность прыжка с полным трюмом.",
        "The shield protects during docking and interdiction escapes. Check jump range at full cargo."],
    "trade/guarded": ["Защищённый тяжёлый фрахт", "Shielded heavy freight",
        "Грузовые отсеки Mk II и FSD (SCO) рассчитаны на перевозку между системами. При перехвате уходите в гиперпрыжок.",
        "Mk II racks and an SCO drive suit intersystem freight. High-wake out of interdictions."],
    "trade/ferry": ["Максимум груза для своего носителя", "Maximum cargo for carrier shuttles",
        "Каждый грузовой отсек занят, щита нет. Подходит для короткого перегона между станцией и своим флотоносцем.",
        "All cargo-capable slots carry racks and the fit is shieldless. Use it for short station-to-carrier trips."],
    exploration: ["Картография и дальние прыжки", "Mapping and long-range travel",
        "Заправляйтесь от звёзд по пути. Сканер поверхности нужен для картографирования, AFMU включайте на остановке.",
        "Scoop along the route. Use the surface scanner for mapping and switch on the AFMU while stopped."],
    "exploration/expedition": ["Дальняя экспедиция", "Deep-space expedition",
        "Запас топлива выбран для серии прыжков, а щит и ремонтное оборудование помогают при посадках и долгом маршруте.",
        "The fuel tank supports multiple jumps, while shields and repair equipment cover landings and long trips."],
    exobiology: ["Экзобиология и посадки", "Exobiology and landings",
        "Щит и сканер поверхности помогают в поиске биосигналов и посадках. Для отбора проб наденьте костюм «Артемида».",
        "Shields and the surface scanner support landings and locating biological signals. Wear an Artemis suit to sample them."],
    "laser-mining": ["Лазерная добыча", "Laser mining",
        "Снарядите трюм дронами. Используйте отдельный A-класс разведчика для лазерной добычи и сборщики для обломков.",
        "Load limpets before departure. Use a dedicated A-rated prospector for laser mining and collectors for fragments."],
    "laser-mining/volley": ["Лазерная добыча с залповым повторителем", "Volley repeater laser mining",
        "Залповый повторитель и лазеры работают вместе с контроллером дронов Mk II. Назначьте разведчика и сборщиков раздельно.",
        "Pair the volley repeater and lasers with the Mk II limpet controller. Put prospectors and collectors in separate groups."],
    "core-mining": ["Добыча из ядер астероидов", "Deep-core mining",
        "Импульсный анализатор ищет кандидатов. Сейсмические заряды вскрывают ядро, абразивный бластер сбивает отложения.",
        "Find candidates with the pulse-wave analyser, open a core with seismic charges and free deposits with the abrasion blaster."],
    passengers: ["Массовые пассажирские рейсы", "High-capacity passenger transport",
        "Салоны экономкласса для массовых перевозок. Перед вылетом проверьте вместимость и требуемый класс по контракту.",
        "Economy cabins carry groups. Check capacity and required cabin class against the contract before departure."],
    "passengers/luxury": ["VIP и туристические рейсы", "VIP and sightseeing",
        "Самый крупный салон отведён под VIP, оставшиеся — под других пассажиров. Сверяйте приватность по контракту.",
        "The largest cabin is for VIPs, with the others taking additional passengers. Check the contract for privacy requirements."],
    rescue: ["Пассажирская эвакуация", "Passenger evacuation",
        "Салоны экономкласса рассчитаны на массовую эвакуацию. Оставьте теплоотводы в отдельной группе для выхода из горячей зоны.",
        "Economy cabins support mass evacuation. Keep heat sinks in their own group for departures from hot areas."],
    salvage: ["Спасение груза и сбор обломков", "Salvage and recovery",
        "Сборщики освобождают время на маневрирование, а свободный трюм вмещает найденный груз. Возьмите дроны перед вылетом.",
        "Collector limpets leave you free to manoeuvre, and cargo racks hold recovered items. Buy limpets before departure."],
    racing: ["Скоростная трасса", "Racing and speed",
        "Малая масса и форсированные двигатели для трассы и коротких перелётов. Перед гонкой уберите груз.",
        "A lightweight hull with enhanced engines for courses and short transfers. Carry no cargo while racing."],
};

const provenance = {
    edastro: {title: "EDAstro", referenceUrl: "https://edastro.com/exploration/"},
    ptn: {title: "Pilots Trade Network", referenceUrl: "https://pilotstradenetwork.com/ship-builds/anti-xeno/"},
    blackbox: {title: "E:D Black Box", referenceUrl: "https://edblackbox.com/guides/ships/ship-dossiers/panther-clipper-mk-ii-trading.html"},
};

function modules(fit, slot) {
    return engine.candidates(fit, slot.key);
}

function set(fit, slot, module, options = {}) {
    return engine.edit(fit, slot.key, {moduleId: module.id, on: true, priority: 1, ...options});
}

function tune(fit, key, blueprint, grade, experimental) {
    const found = engine.blueprints(fit, key).find(item => item.symbol === blueprint);
    assert(found?.grades.includes(grade), `${fit.shipId} ${key}: missing ${blueprint} G${grade}`);
    const engineering = {blueprint, grade, quality: 1};
    if (experimental) engineering.experimental = experimental;
    return engine.edit(fit, key, {engineering});
}

function mainModule(fit, key, select) {
    const slot = engine.slots(fit).find(item => item.key === key);
    const module = select(modules(fit, slot), slot.size);
    assert(module, `${fit.shipId} ${key}: no compatible core module`);
    return set(fit, slot, module);
}

function matching(group, rating, name) {
    return item => item.group === group && !item.preEngineered &&
        (!rating || item.rating === rating) && (!name || name.test(item.name));
}

function put(fit, kind, accepts, {small = false, off = false, required = true, minSize = 0} = {}) {
    const choices = engine.slots(fit)
        .filter(slot => slot.kind === kind && !slot.module)
        .flatMap(slot => modules(fit, slot).filter(accepts).filter(item => item.size >= minSize)
            .map(item => ({slot, item})))
        .sort((a, b) => small
            ? a.item.size - b.item.size || a.slot.size - b.slot.size
            : b.item.size - a.item.size || a.slot.size - b.slot.size);
    if (!choices.length) {
        assert(!required, `${fit.shipId}: no compatible open ${kind} slot`);
        return fit;
    }
    const {slot, item} = choices[0];
    return set(fit, slot, item, off ? {on: false, priority: 5} : {});
}

function fill(fit, select) {
    for (const slot of engine.slots(fit).filter(item => item.kind === "optional" && !item.module)) {
        const module = select(modules(fit, slot));
        if (module) fit = set(fit, slot, module);
    }
    return fit;
}

function configureCore(fit, role, variant) {
    const travel = ["exploration", "exobiology", "racing"].includes(role);
    const starter = variant === "starter" || (role === "trade" && ["adder", "hauler", "sidewinder"].includes(fit.shipId));
    fit = mainModule(fit, "FrameShiftDrive", candidates => candidates.find(matching("fsd", "A", /\(SCO\)/)));
    fit = mainModule(fit, "MainEngines", (candidates, size) =>
        candidates.find(matching("t", "A", variant === "speed" || role === "racing" ? /Enhanced/ : /^Thrusters$/)) ||
        candidates.find(item => item.size === size && matching("t", "A", /^Thrusters$/)(item)));
    fit = mainModule(fit, "PowerDistributor", (candidates, size) =>
        candidates.find(item => item.size === size && matching("pd", "A", /^Power Distributor$/)(item)));
    fit = mainModule(fit, "PowerPlant", (candidates, size) =>
        candidates.find(item => item.size === size && matching("pp", "A", /^Power Plant$/)(item)));
    for (const [key, group] of [["LifeSupport", "ls"], ["Radar", "s"]]) {
        fit = mainModule(fit, key, (candidates, size) => candidates.find(item => item.size === size && matching(group, "D")(item)));
    }
    if (starter) return fit;
    fit = tune(fit, "FrameShiftDrive", "FSD_LongRange", 5, "special_fsd_heavy");
    fit = tune(fit, "MainEngines", "Engine_Dirty", 5, "special_engine_overloaded");
    fit = tune(fit, "PowerDistributor", travel ? "PowerDistributor_PriorityEngines" : "PowerDistributor_HighFrequency", 5,
        "special_powerdistributor_fast");
    if (travel) fit = tune(fit, "Radar", "Sensor_LightWeight", 5);
    return fit;
}

function shield(fit, {small = false, biweave = false} = {}) {
    const choices = engine.slots(fit).filter(slot => slot.kind === "optional" && !slot.module)
        .sort((a, b) => small ? a.size - b.size : b.size - a.size);
    for (const slot of choices) {
        const item = modules(fit, slot).find(candidate => candidate.size === slot.size &&
            matching(biweave ? "bsg" : "sg", biweave ? "C" : small ? "D" : "A")(candidate));
        if (!item) continue;
        const shielded = set(fit, slot, item);
        if (engine.stats(shielded).shield >= (small ? 35 : 1)) return shielded;
    }
    throw new Error(`${fit.shipId}: no compatible shield`);
}

function cargo(fit) {
    return fill(fit, candidates => candidates.filter(item => ["cr", "crl"].includes(item.group) &&
        !item.preEngineered && item.rating !== "F" && /^(?:Mk\s*II |MKII )?Cargo Rack$/i.test(item.name))
        .sort((a, b) => (b.cargo ?? 0) - (a.cargo ?? 0) || b.size - a.size || Number(b.group === "crl") - Number(a.group === "crl"))[0]);
}

function explorer(fit, variant) {
    fit = shield(fit, {small: true});
    fit = put(fit, "optional", matching("fs", "A"));
    fit = put(fit, "optional", matching("ss", null, /^Detailed Surface Scanner$/), {small: true});
    fit = put(fit, "utility", matching("hs", null, /^Heat Sink Launcher$/), {small: true});
    if (variant !== "exobiology") {
        fit = put(fit, "optional", matching("gfsb"), {required: false});
        fit = put(fit, "optional", matching("am", "A"), {off: true, required: false});
    }
    fit = put(fit, "optional", matching("pv", "G", /^Planetary Vehicle Hangar$/), {small: true, required: false});
    return fit;
}

function combat(fit, variant) {
    fit = shield(fit, {biweave: variant !== "large"});
    if (variant === "fighter") {
        fit = put(fit, "optional", matching("fh", "D", /^Fighter Hangar$/), {minSize: 5, small: true});
    }
    const hardpoints = engine.slots(fit).filter(slot => slot.kind === "hardpoint").sort((a, b) => b.size - a.size);
    const kineticCount = Math.ceil(hardpoints.length / 2);
    for (const [index, slot] of hardpoints.entries()) {
        const group = index < kineticCount ? (variant === "fragments" ? "fc" : "mc") : variant === "starter" ? "pl" : "bl";
        const available = modules(fit, slot).filter(item => matching(group)(item) && item.mount === "G");
        const module = available.sort((a, b) => b.size - a.size)[0];
        assert(module, `${fit.shipId} ${slot.key}: missing PvE weapon`);
        fit = set(fit, slot, module);
    }
    if (variant !== "starter") {
        const kinetic = hardpoints[kineticCount - 1].key;
        const beam = hardpoints[kineticCount]?.key;
        fit = tune(fit, kinetic, "Weapon_Overcharged", 5, "special_corrosive_shell");
        if (beam) fit = tune(fit, beam, "Weapon_Efficient", 5, "special_thermal_vent");
        fit = tune(fit, "Armour", "Armour_HeavyDuty", 5, "special_armour_chunky");
        fit = put(fit, "utility", matching("sb", "A"), {required: false});
    }
    fit = put(fit, "utility", matching("ch"), {required: false});
    fit = put(fit, "optional", matching("mrp", "D"), {required: false});
    return fill(fit, candidates => candidates.filter(matching("hr", "D")).sort((a, b) => b.size - a.size)[0]);
}

function miner(fit, laser, variant) {
    const hardpoints = engine.slots(fit).filter(slot => slot.kind === "hardpoint").sort((a, b) => b.size - a.size);
    if (laser) {
        if (variant === "volley") {
            fit = put(fit, "hardpoint", matching("mvr", null, /^Mining Volley Repeater$/));
        }
        let lasers = 0;
        for (const slot of hardpoints.filter(slot => slot.size >= 2 && !engine.slots(fit).find(item => item.key === slot.key).module)) {
            const module = modules(fit, slot).find(item => item.size === 2 && item.mount === "F" && matching("ml", null, /^Mining Laser$/)(item));
            if (module) {
                fit = set(fit, slot, module);
                if (++lasers >= (["asp", "keelback"].includes(fit.shipId) ? 2 : 3)) break;
            }
        }
        assert(lasers > 0, `${fit.shipId}: missing medium mining lasers`);
    } else {
        fit = put(fit, "hardpoint", item => item.mount === "F" && matching("scl")(item), {small: true});
        fit = put(fit, "hardpoint", item => item.mount === "F" && matching("abl")(item), {small: true});
        fit = put(fit, "utility", matching("pwa", "A"));
    }
    fit = shield(fit, {small: true});
    fit = put(fit, "optional", matching("rf", "A"), {small: true, minSize: 2});
    if (fit.shipId === "type_11_prospector") {
        const slot = engine.slots(fit).find(item => item.key === "LimpetController01");
        const controller = modules(fit, slot).find(matching("mlc", "A", /^Mk II Mining Multi-Limpet Controller$/));
        assert(controller, "Type-11 Prospector: missing Mk II Mining Multi-Limpet Controller");
        return cargo(set(fit, slot, controller));
    }
    fit = put(fit, "optional", matching("pc", "A"), {small: true});
    fit = put(fit, "optional", matching("cc", "D"), {required: true, minSize: 3});
    if (hardpoints.length >= 5) fit = put(fit, "optional", matching("cc", "D"), {required: false, minSize: 3});
    return cargo(fit);
}

function transport(fit, role, variant) {
    if (variant === "ferry") return cargo(fit);
    fit = shield(fit, {small: true});
    fit = put(fit, "utility", matching("hs", null, /^Heat Sink Launcher$/), {required: false});
    if (role === "rescue") {
        fit = put(fit, "utility", matching("ec"), {required: false});
        fit = put(fit, "optional", matching("fs", "A"), {small: true, minSize: 3, required: false});
    }
    if (role === "passengers" || role === "rescue") {
        if (variant === "luxury") fit = put(fit, "optional", matching("pcq"));
        return fill(fit, candidates => candidates.filter(matching("pce"))
            .sort((a, b) => b.size - a.size || Number(b.rating === "D") - Number(a.rating === "D"))[0]);
    }
    fit = put(fit, "optional", matching("dc", null, /^Advanced Docking Computer$/), {required: false, small: true});
    return cargo(fit);
}

function antiXenoScout(fit) {
    fit = shield(fit, {biweave: true});
    const enhanced = item => matching("axmce", null, /Enhanced/)(item) && item.mount === "G";
    for (let index = 0; index < 3; index++) fit = put(fit, "hardpoint", enhanced);
    fit = put(fit, "utility", matching("xs", null, /^Enhanced Xeno Scanner$/));
    fit = put(fit, "utility", matching("csl"));
    fit = put(fit, "utility", matching("hs", null, /^Heat Sink Launcher$/));
    fit = put(fit, "optional", matching("mrp", "D"));
    return fill(fit, candidates => candidates.filter(matching("hr", "D")).sort((a, b) => b.size - a.size)[0]);
}

function originalFit(shipId, role, variant) {
    let fit = engine.create(shipId);
    for (const slot of engine.slots(fit).filter(slot => !slot.required && slot.module && slot.kind !== "hatch")) {
        fit = engine.edit(fit, slot.key, {moduleId: null});
    }
    fit = configureCore(fit, role, variant);
    switch (role) {
    case "combat":
        return combat(fit, variant);
    case "anti-xeno":
        return antiXenoScout(fit);
    case "trade":
    case "passengers":
    case "rescue":
        return transport(fit, role, variant);
    case "exobiology":
        return explorer(fit, "exobiology");
    case "laser-mining":
    case "core-mining":
        return miner(fit, role === "laser-mining", variant);
    case "salvage":
        fit = shield(fit, {small: true});
        fit = put(fit, "utility", matching("cs", "A"));
        fit = put(fit, "optional", matching("cc", "D"), {minSize: 3});
        return cargo(fit);
    case "racing":
        return shield(fit, {small: true});
    default:
        throw new Error(`No fitting recipe for ${role}`);
    }
}

function explorationFit(fit) {
    // EDAstro supplies hull-specific long-range builds. Use a full main tank in the
    // comparison and keep a heat sink ready even on lightweight postal fits.
    delete fit.fuel;
    delete fit.cargo;
    const slots = engine.slots(fit);
    if (!slots.some(slot => slot.module?.group === "hs")) {
        fit = put(fit, "utility", matching("hs", null, /^Heat Sink Launcher$/), {required: false});
    }
    if (!slots.some(slot => slot.module?.group === "ss")) {
        fit = put(fit, "optional", matching("ss", null, /^Detailed Surface Scanner$/), {small: true});
    }
    if (!slots.some(slot => ["sg", "bsg", "psg"].includes(slot.module?.group))) fit = shield(fit, {small: true});
    for (const slot of slots.filter(item => item.module?.group === "am" && item.on)) {
        fit = engine.edit(fit, slot.key, {on: false, priority: 5});
    }
    return fit;
}

function ptnFit(fit, variant) {
    // Keep the authors' weapon geometry. Replace one of the duplicate sinks with
    // a caustic sink or a long-range xeno scanner where the original lacks it.
    const slots = engine.slots(fit);
    const duplicateSinks = slots.filter(slot => slot.module?.group === "hs");
    if (duplicateSinks.length > 1) {
        const slot = duplicateSinks.at(-1);
        const needed = slots.some(item => item.module?.group === "csl")
            ? matching("xs", null, /^Enhanced Xeno Scanner$/) : matching("csl");
        if (!slots.some(item => item.module && needed(item.module))) {
            const module = modules(fit, slot).find(needed);
            if (module) fit = set(fit, slot, module);
        }
    }
    for (const slot of slots.filter(item => item.module?.group === "am" && item.on)) {
        fit = engine.edit(fit, slot.key, {on: false, priority: 5});
    }
    if (variant === "shards") {
        // The published share code contains modified Gauss cannons in these
        // positions. Use the modified shards specified in the authors' guide.
        for (const slot of slots.filter(item => item.module?.group === "ggc")) {
            const shard = modules(fit, slot).find(item => item.group === "gsc" && item.size === 2 &&
                item.preEngineered && item.name === "Shard (OC+Foc+SPen)");
            assert(shard, "Cobra Mk V: missing modified shard cannon");
            fit = set(fit, slot, shard, {priority: slot.priority});
        }
    }
    return fit;
}

function decodePanther(url) {
    const data = new URL(url).searchParams.get("data");
    const loadout = JSON.parse(gunzipSync(Buffer.from(data, "base64")));
    // The published fit predates the engine's journal-based layout for restricted bays.
    const keys = ["Cargo01", "Slot01_Size8", "Cargo02", "Slot02_Size7", "Slot03_Size6",
        "Slot04_Size6", "Slot05_Size6", "Slot06_Size5", "Slot07_Size5", "Slot08_Size4",
        "Slot09_Size2", "Slot10_Size1"];
    loadout.Modules = loadout.Modules.filter(item => item.Slot !== "PlanetaryApproachSuite").map(item => {
        const index = Number(/^Slot(\d+)/.exec(item.Slot)?.[1]);
        return index ? {...item, Slot: keys[index - 1]} : item;
    });
    let fit = engine.fromLoadout(loadout);
    fit = mainModule(fit, "FrameShiftDrive", candidates => candidates.find(matching("fsd", "A", /\(SCO\)/)));
    fit = tune(fit, "FrameShiftDrive", "FSD_LongRange", 5, "special_fsd_heavy");
    for (const slot of engine.slots(fit).filter(item => /^Cargo0/.test(item.key))) {
        const rack = modules(fit, slot).find(item => item.size === slot.size && matching("crl")(item));
        assert(rack, `Panther: missing Mk II cargo rack for ${slot.key}`);
        fit = set(fit, slot, rack);
    }
    return fit;
}

function finish(fit) {
    let stats = engine.stats(fit);
    if (stats.powerRetracted > stats.powerAvailable || stats.powerDeployed > stats.powerAvailable) {
        const live = Math.max(stats.powerRetracted, stats.powerDeployed);
        for (const grade of [1, 2, 3, 4, 5]) {
            fit = tune(fit, "PowerPlant", "PowerPlant_Boosted", grade, "special_powerplant_cooled");
            if (engine.stats(fit).powerAvailable >= live) break;
        }
        stats = engine.stats(fit);
    }
    assert(stats.powerRetracted <= stats.powerAvailable + 0.001 &&
        stats.powerDeployed <= stats.powerAvailable + 0.001 &&
        stats.ladenRange > 0 && stats.ladenBoostSpeed > 0, `${fit.shipId}: unpowered or cannot travel when loaded`);
    return fit;
}

function createPreset(ship, profile) {
    const [role, variant] = profile.split("/");
    const [name, nameEn, notes, notesEn] = profiles[profile];
    let source = {kind: "original", title: "ED Copilot"};
    let fit;
    if (role === "exploration") {
        const url = seeds.exploration[ship.id];
        fit = explorationFit(engine.parse(url)[0]);
        source = {kind: "adapted", ...provenance.edastro, url};
    } else if (role === "anti-xeno" && seeds.antiXeno[ship.id]) {
        const url = seeds.antiXeno[ship.id];
        fit = ptnFit(engine.parse(url)[0], variant);
        source = {kind: "adapted", ...provenance.ptn, url};
    } else if (variant === "guarded") {
        fit = decodePanther(seeds.panther);
        source = {kind: "adapted", ...provenance.blackbox, url: seeds.panther};
    } else {
        fit = originalFit(ship.id, role, variant);
    }
    assert.equal(fit.shipId, ship.id, `${ship.id}: source URL resolved to a different ship`);
    const title = `${ship.name} · ${name}`;
    const titleEn = `${ship.name} · ${nameEn}`;
    const specific = ship.id === "alliance_chieftain" && role === "anti-xeno"
        ? ["Четыре пушки Гаусса и зенитная установка для соло-перехватчиков. Начните с «Циклопа».",
            "Four Gauss cannons and a flak launcher for solo Interceptors. Start with a Cyclops."]
        : [notes, notesEn];
    return {
        id: [ship.id, role, variant].filter(Boolean).join("-"),
        shipId: ship.id, role, title, titleEn, notes: specific[0], notesEn: specific[1],
        build: {...finish(fit), name: titleEn}, source,
    };
}

const ships = engine.ships();
assert.deepEqual(Object.keys(assignments).sort(), ships.map(ship => ship.id).sort());
const builds = ships.flatMap(ship => assignments[ship.id].map(profile => {
    try {
        return createPreset(ship, profile);
    } catch (error) {
        throw new Error(`${ship.id} / ${profile}: ${error.message}`, {cause: error});
    }
}));
const output = "// Generated by scripts/ship-fit/presets.mjs.\n" +
    "globalThis.EDCopilotShipFitPresets = {\"version\":1,\"builds\":[\n" +
    builds.map(item => JSON.stringify(item)).join(",\n") + "\n]};\n";
const target = new URL("../../internal/frontend/ship-fit-presets.js", import.meta.url);
if (process.argv.includes("--check")) {
    assert.equal(readFileSync(target, "utf8").replaceAll("\r\n", "\n"), output, "Regenerate the shipped ship fits");
} else {
    writeFileSync(target, output);
}
