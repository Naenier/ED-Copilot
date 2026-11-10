(() => {
'use strict';
const labels = {
  partial:['Incomplete','Неполные данные'], empty:['Empty result','Пустой результат'],
};
const reasons = {
  offline:['No connection to the data provider.','Нет соединения с источником данных.'],
  'rate-limited':['The provider is limiting requests. Copilot will retry automatically.','Источник ограничил частоту запросов. Copilot повторит попытку автоматически.'],
  'no-match':['The public index has no exact match for this system.','В открытой базе нет точного совпадения для этой системы.'],
  incomplete:['The provider returned incomplete data.','Источник вернул неполные данные.'],
  unavailable:['Could not update these observations.','Не удалось обновить данные.'],
};
function pick(pair,lang) { return pair[lang === 'ru' ? 1 : 0]; }
function reason(code,lang) { return pick(reasons[code] || reasons.unavailable,lang); }
function details(label,lines,esc,warning=false) {
  if (!lines.length) return '<span class="freshness-badge'+(warning?' is-warning':'')+'">'+esc(label)+'</span>';
  return '<details class="freshness-detail'+(warning?' is-warning':'')+'"><summary>'+esc(label)+'</summary><dl>'+
    lines.map(([term,value])=>'<div><dt>'+esc(term)+'</dt><dd>'+esc(value)+'</dd></div>').join('')+'</dl></details>';
}
function formatDate(value,lang) {
  return Number.isFinite(Date.parse(value)) ? new Date(value).toLocaleString(lang,{dateStyle:'medium',timeStyle:'short'}) : '';
}
function observation(quality,lang,esc) {
  if (!quality) return '';
  const lines = [];
  for (const [field,name] of [['observedAt',['Observed','Наблюдение']],['fetchedAt',['Retrieved','Получение']]]) {
    const stamp = formatDate(quality[field],lang);
    if (stamp) lines.push([pick(name,lang),stamp]);
  }
  const coverage = labels[quality.coverage];
  if (!coverage && !lines.length) return '';
  return details(pick(coverage || ['Data dates','Даты данных'],lang),lines,esc,quality.coverage === 'partial');
}
window.FreshnessUI = {details,formatDate,observation,reason};
})();
