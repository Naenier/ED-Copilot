'use strict';
const TAB_IDS = Object.freeze(['overview', 'targets', 'sale', 'deals', 'carrier', 'colonization', 'encyclopedia', 'statistics', 'mining', 'engineering', 'route', 'settings', 'account', 'integrations', 'help']);
const DEFAULT_TABS = Object.freeze(['overview', 'targets', 'sale', 'encyclopedia', 'statistics', 'settings']);
function normalizeNavigation(value, strict = false) {
  if (value == null && !strict) return [...DEFAULT_TABS];
  if (!Array.isArray(value) || value.length > TAB_IDS.length || value.some(id => typeof id !== 'string' || !TAB_IDS.includes(id))) {
    if (!strict) return [...DEFAULT_TABS];
    throw new Error('Выберите вкладки из списка доступных разделов.');
  }
  const tabs = [...new Set(value)];
  if (!tabs.includes('overview')) tabs.unshift('overview');
  if (!tabs.includes('settings')) tabs.push('settings');
  return tabs;
}
module.exports = { TAB_IDS, DEFAULT_TABS, normalizeNavigation };
