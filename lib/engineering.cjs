"use strict";

const crypto = require("node:crypto");
const data = require("./engineering-data.json");
const categoryNames = { raw: "Сырьё", manufactured: "Производственные материалы", encoded: "Данные" };
const norm = (value) => String(value ?? "").replace(/^\$/, "").replace(/_name;?$/i, "").toLowerCase().replace(/[^a-zа-яё0-9]/g, "");
const category = (value) => { const text = norm(value); return ["raw", "manufactured", "encoded"].find((key) => text.includes(key)) ?? null; };
const integer = (value, label, min = 0, max = 1000000) => {
  const number = value === "" || value == null ? NaN : Number(value);
  if (!Number.isSafeInteger(number) || number < min || number > max) throw new Error(`${label}: требуется целое число от ${min} до ${max}.`);
  return number;
};
const copy = (value) => JSON.parse(JSON.stringify(value));
const materials = new Map(data.materials.map((row) => [row.key, row]));
const aliases = new Map();
for (const material of data.materials) for (const name of [material.key,material.symbol,material.english,material.name]) if(norm(name))aliases.set(norm(name),material.key);
aliases.set(norm("Adaptive Encyptors Capture"),"adaptiveencryptors");
const materialKey = (value) => norm(value) ? aliases.get(norm(value)) ?? null : null;
const blueprints = new Map(data.blueprints.map((row) => [row.key,row]));
const modules = new Map(data.modules.map((row) => [row.key,row]));
const effects = new Map(data.experimentals.map((row) => [row.key,row]));
const engineers = new Map(data.engineers.map((row) => [row.key,row]));
const familyNames = new Map();
for (const material of [...data.materials].sort((a,b)=>a.grade-b.grade)) {
  const familyKey=material.category+":"+material.family;
  if(material.family&&!familyNames.has(familyKey))familyNames.set(familyKey,material.name);
}

function validateItem(input, id = input?.id ?? crypto.randomUUID()) {
  if (!input || typeof input !== "object") throw new Error("Нужны параметры улучшения.");
  const module = modules.get(input.moduleKey);
  const blueprint = blueprints.get(input.blueprintKey);
  if (!module || !blueprint || !module.blueprints.includes(blueprint.key)) throw new Error("Выберите совместимые модуль и чертёж.");
  let rolls = input.rolls;
  if (!Array.isArray(rolls)) rolls = [1,2,3,4,5].map((g) => input.rolls?.[g] ?? 0);
  if (rolls.length > 5) throw new Error("Допускаются только пять ступеней улучшения.");
  rolls = Array.from({length:5},(_,i) => integer(rolls[i] ?? 0, `Попытки G${i+1}`,0,100));
  for (let index=0;index<5;index++) if (rolls[index] && !blueprint.grades[String(index+1)]) throw new Error(`Для этого чертежа нет ступени G${index+1}.`);
  const experimentalKey = input.experimentalKey || null;
  if (experimentalKey && (!effects.has(experimentalKey) || !module.experimentals.includes(experimentalKey))) throw new Error("Экспериментальный эффект несовместим с модулем.");
  if (!rolls.some(Boolean) && !experimentalKey) throw new Error("Укажите хотя бы одну попытку или экспериментальный эффект.");
  return { id:String(id).slice(0,80),moduleKey:module.key,blueprintKey:blueprint.key,rolls,experimentalKey,
    quantity:integer(input.quantity ?? 1,"Количество модулей",1,100),label:String(input.label ?? "").trim().slice(0,150) };
}

function itemMaterials(item) {
  const needed = new Map();
  const add = (recipe,multiplier) => {for(const entry of recipe)needed.set(entry.key,(needed.get(entry.key)||0)+entry.count*multiplier);};
  item.rolls.forEach((rolls,index)=>{if(rolls)add(blueprints.get(item.blueprintKey).grades[index+1],rolls*item.quantity);});
  if(item.experimentalKey)add(effects.get(item.experimentalKey).materials,item.quantity);
  return [...needed].map(([key,count])=>({key,name:materials.get(key).name,count}));
}

function gcd(a,b) {while(b){const rem=a%b;a=b;b=rem;}return a;}
function tradeRatio(from,to) {
  if(!from||!to||from.key===to.key||!from.tradeable||!to.tradeable||from.category!==to.category)return null;
  const difference=to.grade-from.grade;
  let cost=difference>0?6**difference:1;
  let gain=difference<0?3**(-difference):1;
  if(from.family!==to.family)cost*=6;
  const divisor=gcd(cost,gain);
  return {cost:cost/divisor,gain:gain/divisor};
}

function tradeOffer(from,to,wanted,available,targetOwned=null,protectedCount=0) {
  const ratio=tradeRatio(from,to);if(!ratio)return null;
  const batches=Math.ceil(wanted/ratio.gain);
  const space=targetOwned==null?null:Math.max(0,to.capacity-targetOwned);
  const storageBatches=space==null?null:Math.floor(space/ratio.gain);
  const affordableBatches=available==null?null:Math.floor(available/ratio.cost);
  const maxBatches=storageBatches==null?affordableBatches:affordableBatches==null?null:Math.min(storageBatches,affordableBatches);
  const requiredFrom=batches*ratio.cost,received=batches*ratio.gain;
  return {fromKey:from.key,toKey:to.key,fromName:from.name,toName:to.name,category:from.category,
    fromGrade:from.grade,toGrade:to.grade,sameFamily:from.family===to.family,...ratio,
    requested:wanted,batches,requiredFrom,received,available,protectedCount,
    affordable:available==null?null:available>=requiredFrom,
    storageFits:space==null?null:space>=received,targetSpace:space,
    maxGain:maxBatches==null?null:maxBatches*ratio.gain,maxCost:maxBatches==null?null:maxBatches*ratio.cost,
    stockKnown:available!=null,targetStockKnown:targetOwned!=null};
}

class EngineeringPlanner {
  constructor(saved={}) {
    this.items=[];this.options={tradeTargetKey:"",tradeTargetAmount:1,protectPlan:true,showAllTrades:false};this.manualEngineers={};
    if(Array.isArray(saved.items))for(const item of saved.items.slice(0,200)){try{this.items.push(validateItem(item));}catch{}}
    if(saved.options){try{this.configure(saved.options);}catch{}}
    if(saved.manualEngineers&&typeof saved.manualEngineers==='object')for(const [key,value]of Object.entries(saved.manualEngineers)){
      if(engineers.has(key)&&typeof value==='boolean')this.manualEngineers[key]=value;
    }
    this.reset();
  }
  reset() {
    this.stock=new Map();this.categoryKnown={raw:false,manufactured:false,encoded:false};this.uncertain=new Set();
    this.inventoryAt={raw:null,manufactured:null,encoded:null};this.progress=new Map();this.lastEventAt=null;this.warnings=[];
  }
  configure(input={}) {
    const next={...this.options};
    if(input.tradeTargetKey!=null){const key=input.tradeTargetKey?materialKey(input.tradeTargetKey):"";if(input.tradeTargetKey&&!key)throw new Error("Материал не найден.");next.tradeTargetKey=key;}
    if(input.tradeTargetAmount!=null)next.tradeTargetAmount=integer(input.tradeTargetAmount,"Нужное количество",1,100000);
    for(const field of ["protectPlan","showAllTrades"])if(typeof input[field]==='boolean')next[field]=input[field];
    this.options=next;return this.exportSettings();
  }
  add(input) {if(this.items.length>=200)throw new Error("В плане допускается до 200 улучшений.");const item=validateItem(input,crypto.randomUUID());this.items.push(item);return item;}
  update(input) {const index=this.items.findIndex((item)=>item.id===input?.id);if(index<0)throw new Error("Улучшение не найдено.");const next=validateItem({...this.items[index],...input},this.items[index].id);this.items[index]=next;return next;}
  remove(id) {const index=this.items.findIndex((item)=>item.id===String(id));if(index<0)throw new Error("Улучшение не найдено.");this.items.splice(index,1);}
  setEngineer(input={}) {
    const key=String(input.key ?? input.id ?? "");if(!engineers.has(key))throw new Error("Инженер не найден.");
    if(input.unlocked==null)delete this.manualEngineers[key];else if(typeof input.unlocked==='boolean')this.manualEngineers[key]=input.unlocked;
    else throw new Error("Статус инженера должен быть отмечен или снят.");
  }
  replaceItems(input) {
    const rows=Array.isArray(input)?input:input?.items;
    if(!Array.isArray(rows)||rows.length>200)throw new Error("План должен содержать до 200 улучшений.");
    const next=rows.map((item)=>validateItem(item,crypto.randomUUID()));
    this.items=next;return this.items;
  }
  importPlan(input) {return this.replaceItems(input);}
  exportPlan() {return {format:"ed-mining-engineering",version:1,items:this.items.map(({id,...item})=>copy(item))};}
  exportSettings() {return {items:copy(this.items),options:{...this.options},manualEngineers:{...this.manualEngineers}};}
  changeStock(name,delta,event) {
    const key=materialKey(name);if(!key||!Number.isFinite(delta)||delta===0)return;
    const material=materials.get(key);const previous=this.stock.get(key)||0;
    if(this.categoryKnown[material.category]&&previous+delta<0)this.uncertain.add(key);
    this.stock.set(key,Math.max(0,previous+delta));this.inventoryAt[material.category]=event.timestamp||this.inventoryAt[material.category];
  }
  apply(event) {
    if(!event||typeof event.event!=='string')return;
    this.lastEventAt=event.timestamp||this.lastEventAt;
    if(event.event==='Materials')for(const [field,group]of [['Raw','raw'],['Manufactured','manufactured'],['Encoded','encoded']]){
      if(!Array.isArray(event[field]))continue;
      for(const material of data.materials)if(material.category===group){this.stock.delete(material.key);this.uncertain.delete(material.key);}
      for(const row of event[field]){const key=materialKey(row.Name);if(key&&materials.get(key).category===group&&Number.isSafeInteger(row.Count)&&row.Count>=0)this.stock.set(key,row.Count);}
      this.categoryKnown[group]=true;this.inventoryAt[group]=event.timestamp||null;
    }
    if(['MaterialCollected','MaterialDiscarded'].includes(event.event))this.changeStock(event.Name,(event.event==='MaterialDiscarded'?-1:1)*Math.max(0,Number(event.Count)||0),event);
    if(event.event==='MaterialTrade'){
      this.changeStock(event.Paid?.Material??event.Paid?.Name,-Math.max(0,Number(event.Paid?.Quantity??event.Paid?.Count)||0),event);
      this.changeStock(event.Received?.Material??event.Received?.Name,Math.max(0,Number(event.Received?.Quantity??event.Received?.Count)||0),event);
    }
    if(event.event==='EngineerContribution'&&String(event.Type).toLowerCase()==='materials')this.changeStock(event.Material,-Math.max(0,Number(event.Quantity)||0),event);
    if(event.event==='MissionCompleted')for(const row of event.MaterialsReward||[])this.changeStock(row.Name,Math.max(0,Number(row.Count)||0),event);
    const consumption=event.event==='EngineerCraft'?event.Ingredients:['Synthesis','TechnologyBroker'].includes(event.event)?event.Materials:null;
    if(Array.isArray(consumption))for(const row of consumption)this.changeStock(row.Name,-Math.max(0,Number(row.Count)||0),event);
    if(event.event==='EngineerProgress')for(const row of Array.isArray(event.Engineers)?event.Engineers:[event]){
      const match=String(row.EngineerID??'');const byName=data.engineers.find((e)=>norm(e.english)===norm(row.Engineer));
      const key=engineers.has(match)?match:byName?.key;if(!key)continue;
      const previous=this.progress.get(key)||{};
      this.progress.set(key,{...previous,rank:Number.isFinite(Number(row.Rank))?Number(row.Rank):previous.rank??null,
        progress:Number.isFinite(Number(row.RankProgress))?Number(row.RankProgress):previous.progress??null,
        status:row.Progress?String(row.Progress):Number(row.Rank)>0?'Unlocked':previous.status??'Unknown',timestamp:event.timestamp||null});
    }
  }
  snapshot() {
    const totals=new Map();
    const items=this.items.map((item)=>{
      const recipe=itemMaterials(item);for(const row of recipe)totals.set(row.key,(totals.get(row.key)||0)+row.count);
      const module=modules.get(item.moduleKey),blueprint=blueprints.get(item.blueprintKey);
      const lastGrade=item.rolls.reduce((value,rolls,index)=>rolls?index+1:value,0);
      const engineerNames=module.engineers[item.blueprintKey]?.[Math.max(lastGrade,1)]||[];
      return {...copy(item),moduleName:module.name,blueprintName:blueprint.name,experimentalName:item.experimentalKey?effects.get(item.experimentalKey).name:null,
        maxGrade:lastGrade,materials:recipe,engineerKeys:data.engineers.filter((e)=>engineerNames.includes(e.english)).map((e)=>e.key)};
    });
    const rows=data.materials.map((material)=>{
      const owned=this.categoryKnown[material.category]&&!this.uncertain.has(material.key)?this.stock.get(material.key)||0:null;
      const needed=totals.get(material.key)||0;
      return {...material,categoryName:categoryNames[material.category],familyName:material.family?familyNames.get(material.category+":"+material.family):"Вне обменных групп",owned,needed,
        missing:owned==null?null:Math.max(0,needed-owned),surplus:owned==null?null:Math.max(0,owned-needed),
        updatedAt:this.inventoryAt[material.category]};
    });
    const target=rows.find((row)=>row.key===this.options.tradeTargetKey)||null;
    const offers=target?rows.filter((row)=>row.category===target.category&&row.tradeable).map((row)=>tradeOffer(row,target,this.options.tradeTargetAmount,
      this.options.protectPlan?row.surplus:row.owned,target.owned,this.options.protectPlan?row.needed:0)).filter(Boolean)
      .filter((offer)=>this.options.showAllTrades||offer.available==null||offer.maxGain>0)
      .sort((a,b)=>Number(b.affordable&&b.storageFits!==false)-Number(a.affordable&&a.storageFits!==false)||Number(b.sameFamily)-Number(a.sameFamily)||a.requiredFrom-b.requiredFrom):[];
    const needed=rows.filter((row)=>row.needed>0);
    const engineerRows=data.engineers.map((engineer)=>{
      const journal=this.progress.get(engineer.key)||{};
      const manualUnlocked=this.manualEngineers[engineer.key]??null;
      const journalStatus=String(journal.status||'unknown').toLowerCase();
      const status=manualUnlocked===true?'unlocked':manualUnlocked===false?'locked':['unlocked','invited','known','acquainted','barred'].includes(journalStatus)?journalStatus:'unknown';
      const capabilities=[];
      for(const module of data.modules)for(const [blueprintKey,grades]of Object.entries(module.engineers)){
        const maxGrade=Math.max(0,...Object.entries(grades).filter(([,names])=>names.includes(engineer.english)).map(([grade])=>Number(grade)));
        if(maxGrade)capabilities.push({moduleKey:module.key,moduleName:module.name,blueprintKey,blueprintName:blueprints.get(blueprintKey).name,maxGrade});
      }
      return {...engineer,status,manualUnlocked,journalStatus:journal.status||null,rank:journal.rank??null,progress:journal.progress??null,
        updatedAt:journal.timestamp||null,capabilities};
    });
    return {catalog:data,items,materials:rows,shopping:{neededUnits:needed.reduce((sum,row)=>sum+row.needed,0),
        missingUnits:needed.every((row)=>row.owned!=null)?needed.reduce((sum,row)=>sum+row.missing,0):null,
        unknownMaterials:needed.filter((row)=>row.owned==null).length,
      inventoryKnown:needed.every((row)=>row.owned!=null),complete:needed.every((row)=>row.owned!=null&&row.missing===0),
      knownCategories:{...this.categoryKnown},updatedAt:{...this.inventoryAt}},
      trader:{targetKey:this.options.tradeTargetKey,targetName:target?.name||'',targetAmount:this.options.tradeTargetAmount,
        protectPlan:this.options.protectPlan,showAllTrades:this.options.showAllTrades,offers},engineers:engineerRows,
      engineerSummary:{unlocked:engineerRows.filter((row)=>row.status==='unlocked').length,total:engineerRows.length},
      options:{...this.options},sources:data.sources,
      help:{rolls:'Количество попыток задаётся отдельно для каждой ступени. При максимальной репутации инженера полные ступени требуют 1/2/3/4/5 попыток; при меньшей репутации задайте больше. Ноль пропускает уже пройденную ступень. Экспериментальный эффект учитывается один раз на каждый модуль.',
        inventory:'Запасы берутся из полных снимков Materials и последующих событий журнала. До снимка категории запас неизвестен. Изменение плана не расходует материалы в игре.',
        trader:'Обмен возможен только внутри типа материалов. В одной группе: 1→3 на ступень вниз, 6→1 вверх. Другая группа добавляет множитель стоимости 6. Обмен выполняется целыми партиями; избыток и предел хранилища учитываются. Варианты альтернативные и используют один и тот же запас.',
        engineers:'Статусы и репутация поступают из EngineerProgress. Ручная отметка имеет приоритет; её можно сбросить к журналу. План охватывает корабельные модули, а не снаряжение Odyssey.',
        data:'Рецепты и совместимость взяты из EDCD/Coriolis. Чертежи и старые экспериментальные эффекты без известного состава не предлагаются; современные варианты с известной стоимостью доступны. Проверяйте фактический рецепт в игре при изменении механик.'}};
  }
}

module.exports={EngineeringPlanner,validateItem,itemMaterials,tradeRatio,tradeOffer,materialKey,data,category};
