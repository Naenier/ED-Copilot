const assert = require("node:assert/strict");
const test = require("node:test");
require("./ship-fit-engine.js");
require("./ship-fit-presets.js");

const engine = globalThis.EDCopilotShipFitEngine;
const {version, builds} = globalThis.EDCopilotShipFitPresets;

test("the preset library covers every catalogue hull with distinct native builds", () => {
    assert.equal(version, 1);
    assert.deepEqual([...new Set(builds.map(preset => preset.shipId))].sort(), engine.ships().map(ship => ship.id).sort());
    assert.equal(new Set(builds.map(preset => preset.id)).size, builds.length);
    assert.equal(new Set(builds.map(preset => `${preset.shipId}/${preset.build.code}`)).size, builds.length);
    assert.ok(builds.filter(preset => preset.shipId === "python").length > 1);
    assert.ok(builds.filter(preset => preset.shipId === "cobramkv").length > 1);
    for (const preset of builds) {
        assert.ok(preset.title && preset.titleEn && preset.notes && preset.notesEn, preset.id);
        const [roundTrip] = engine.parse(engine.export(preset.build, "native"));
        assert.equal(roundTrip.shipId, preset.shipId, preset.id);
        assert.deepEqual(roundTrip, preset.build, preset.id);
        const stats = engine.stats(preset.build);
        assert.ok(stats.ladenRange > 0 && stats.ladenBoostSpeed > 0, preset.id);
        assert.ok(stats.powerAvailable >= stats.powerRetracted - 1e-3, preset.id);
        assert.ok(stats.powerAvailable >= stats.powerDeployed - 1e-3, preset.id);
        if (preset.source.kind === "adapted") {
            assert.equal(new URL(preset.source.url).origin, "https://coriolis.io", preset.id);
            assert.equal(new URL(preset.source.referenceUrl).protocol, "https:", preset.id);
        } else {
            assert.equal(preset.source.kind, "original", preset.id);
        }
    }
});

test("each preset carries the equipment required by its role", () => {
    for (const preset of builds) {
        const stats = engine.stats(preset.build);
        const slots = engine.slots(preset.build);
        const has = (...groups) => slots.some(slot => groups.includes(slot.module?.group));
        switch (preset.role) {
        case "anti-xeno":
            assert.ok(has("ggc", "gsc", "gpc", "axmce") && has("hs") && has("mrp", "gmrp"), preset.id);
            break;
        case "combat":
            assert.ok(stats.dps > 0 && stats.shield > 0, preset.id);
            if (preset.id.endsWith("-fighter")) assert.ok(has("fh"), preset.id);
            if (preset.id.endsWith("-starter")) assert.ok(slots.every(slot => !slot.engineering), preset.id);
            break;
        case "trade":
            assert.ok(stats.cargoCapacity > 0, preset.id);
            break;
        case "exploration":
        case "exobiology":
            assert.ok(stats.shield > 0 && has("ss") && has("fs") && has("hs"), preset.id);
            break;
        case "laser-mining":
        case "core-mining":
            assert.ok(stats.cargoCapacity > 0 && has("rf") && (has("mlc") || has("cc") && has("pc")), preset.id);
            if (preset.role === "laser-mining") assert.ok(has("ml"), preset.id);
            else assert.ok(has("abl") && has("scl") && has("pwa"), preset.id);
            break;
        case "passengers":
        case "rescue":
            assert.ok(stats.passengerCapacity > 0 && has("hs"), preset.id);
            if (preset.id.endsWith("-luxury")) assert.ok(has("pcq"), preset.id);
            break;
        case "salvage":
            assert.ok(stats.cargoCapacity > 0 && has("cc") && has("cs"), preset.id);
            break;
        case "racing":
            assert.ok(stats.ladenBoostSpeed > 750 && has("sg"), preset.id);
            break;
        default:
            assert.fail(`Unexpected preset role ${preset.role}`);
        }
    }
});

test("Chieftain has the solo Gauss fit and Panther has two usable freighters", () => {
    const find = id => builds.find(preset => preset.id === id);
    const chief = find("alliance_chieftain-anti-xeno");
    const slots = engine.slots(chief.build);
    assert.equal(chief.source.title, "Pilots Trade Network");
    assert.equal(slots.filter(slot => slot.module?.group === "ggc" && slot.on).length, 4);
    assert.ok(slots.some(slot => slot.module?.group === "rfl" && slot.on));
    assert.ok(slots.some(slot => slot.module?.group === "bl" &&
        slot.engineering?.experimental === "special_thermal_vent"));
    assert.ok(slots.some(slot => slot.module?.group === "csl"));

    const guarded = find("panthermkii-trade-guarded");
    const ferry = find("panthermkii-trade-ferry");
    assert.equal(guarded.source.title, "E:D Black Box");
    for (const preset of [guarded, ferry]) {
        const reserved = engine.slots(preset.build).filter(slot => /^Cargo0/.test(slot.key));
        assert.equal(reserved.length, 2);
        assert.ok(reserved.every(slot => slot.module?.group === "crl" && slot.module.size === slot.size));
    }
    const freight = engine.stats(guarded.build);
    const shuttle = engine.stats(ferry.build);
    assert.ok(freight.cargoCapacity >= 1100 && freight.shield > 0);
    assert.ok(shuttle.cargoCapacity >= 1200 && shuttle.cargoCapacity > freight.cargoCapacity);
    assert.equal(shuttle.shield, 0);
});
