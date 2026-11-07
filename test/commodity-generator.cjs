"use strict";
const assert = require("node:assert/strict");
const { parseCsv, buildMap } = require("../scripts/generate-commodity-names.cjs");
const rows = parseCsv('id,symbol,category,name\r\n1,TestItem,Example,"A, quoted ""name"""\r\n2,SimpleItem,Example,Simple Item\r\n');
assert.equal(rows[0].name, 'A, quoted "name"');
const { mapping, supplemental } = buildMap(rows, [{ kind: "commodity", key: "planetthing", english: "Planet Thing", methods: ["surface-planet"] }]);
assert.deepEqual(mapping.testitem, ['A, quoted "name"', "TestItem"]);
assert.deepEqual(mapping.aquotedname, mapping.testitem);
assert.deepEqual(mapping.simpleitem, ["Simple Item", "SimpleItem"]);
assert.deepEqual(mapping.planetthing, ["Planet Thing", "planetthing"]);
assert.equal(supplemental, 1);
assert.throws(() => parseCsv('id,symbol,name\n1,Test,"unterminated'), /Unterminated/);
const aliases = buildMap([{ symbol: "KnownSymbol", name: "Other" }, { symbol: "Other", name: "Actual Other" }]);
assert.deepEqual(aliases.mapping.other, ["Actual Other", "Other"], "A name alias must not overwrite a canonical journal symbol");
const liveMap = require("../lib/commodity-names.json");
for (const key of ["platinum", "tritium", "thehuttonmug", "lavianbrandy", "helium3", "thortveitite"]) {
  assert(Array.isArray(liveMap[key]) && liveMap[key].length === 2 && liveMap[key].every(value => typeof value === "string" && value.length));
}
console.log("Commodity generator: CSV quoting, canonical/alias precedence, planetary supplement and packaged reference coverage passed.");
