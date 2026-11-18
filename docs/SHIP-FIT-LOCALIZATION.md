# Russian names in the ship fitting editor

The fitting editor uses Elite Dangerous outfitting names for modules and the workshop names for engineering blueprints and experimental effects. It looks up those names by Frontier symbol and outfitting ID. Hull names are the shipyard model names. The Russian shipyard retains model names such as Panther Clipper Mk II and Cobra Mk V. The outfitting display also retains class and rating codes such as 5A.

The upstream Coriolis catalogue has distinct IDs for some pre-engineered modules which share their base module's Frontier symbol. The editor uses those IDs to resolve names such as «Двигатель FSD (ПГР) V1». Where the game uses an identical display name for a reward and a standard module, the fitting editor preserves the name and identifies the pre-engineered version separately.

The module names favor the full outfitting label over the shorter form in the cockpit panel. For example, the panel and game journal use «Груз. стеллаж», while the editor uses «Грузовой стеллаж». A comparison with the Russian journal from Elite Dangerous 4.4.2.1 covered 119 module symbols. The journal chiefly supplies the shorter panel labels. It also confirms «Крупный гараж для планет. транспорта Mk II» for the class 4 hangar, which the Almanac snapshot had inferred from the original hangar name. The source documents uncommon spelling differences within armor families.

## Regenerating

Use [Elite Dangerous Almanac Core at 42e874e](https://github.com/Elite-Dangerous-Almanac/Almanac-Core/tree/42e874ed8f8b2ad89f99b9a21cc575851415f646) and [Coriolis Data at 0db9234](https://github.com/EDCD/coriolis-data/tree/0db9234b5b9ce8c939ea84133d7ce336eea88e27).

```text
node scripts/ship-fit/localize-ru.mjs <Almanac-Core checkout> <coriolis-data checkout>
```

The generator checks all 960 outfitting module rows from this Coriolis snapshot, 236 hull-specific armor rows, 47 hulls, 81 blueprints and 91 experimental effects. It includes Russian labels for 29 pre-engineered rows. The ten Coriolis stand-ins for modules missing from its catalogue receive Russian placeholder labels. Obsolete experimental-effect IDs resolve to their corresponding names in the current game. The Russian names also cover the Lynx Highliner and the Anaconda Mk II and their additional modules through the Almanac catalogue. The bundle runs locally and contains no data from a player's journal.

The names and game data belong to Frontier Developments plc. ED Copilot uses the data for its non-commercial Elite Dangerous companion app under the applicable Frontier conditions. [Frontier media usage rules](https://forums.frontier.co.uk/threads/elite-dangerous-media-usage-rules.510879/post-7773248) · [Almanac localization sources](https://github.com/Elite-Dangerous-Almanac/Almanac-Core/blob/42e874ed8f8b2ad89f99b9a21cc575851415f646/data/i18n/SOURCES.md) · [Almanac attribution](https://github.com/Elite-Dangerous-Almanac/Almanac-Core/blob/42e874ed8f8b2ad89f99b9a21cc575851415f646/ATTRIBUTIONS.md)
