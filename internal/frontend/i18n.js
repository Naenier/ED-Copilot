/* Explicit UI translation. Entity names and entered values never pass through this layer. */
(() => {
  'use strict';
  let language = 'ru';
  const numbers = new Map(), dates = new Map(), plurals = new Map();
  const normalize = value => value === 'en' ? 'en' : 'ru';
  const locale = () => language === 'en' ? 'en-GB' : 'ru-RU';
  const interpolate = (value, variables) => String(value ?? '').replace(/\{([\w]+)\}/g, (match, key) =>
    variables && Object.hasOwn(variables, key) ? String(variables[key] ?? '') : match);
  function t(source, variables) {
    const key = String(source ?? ''), catalogue = window.CopilotTranslations || {};
    const value = language === 'en' && Object.hasOwn(catalogue, key) && typeof catalogue[key] === 'string' ? catalogue[key] : key;
    return interpolate(value, variables);
  }
  function setLanguage(value) {
    const next = normalize(value), changed = next !== language;
    language = next;
    if (typeof document !== 'undefined' && document.documentElement) document.documentElement.lang = language;
    return changed;
  }
  function number(value, options = {}) {
    if (value === null || value === undefined || value === '' || !Number.isFinite(Number(value))) return '—';
    const key = locale() + JSON.stringify(options);
    if (!numbers.has(key)) numbers.set(key, new Intl.NumberFormat(locale(), options));
    return numbers.get(key).format(Number(value));
  }
  function date(value, options = {}) {
    const parsed = value instanceof Date ? value : new Date(value);
    if (value === null || value === undefined || value === '' || !Number.isFinite(parsed.getTime())) return '—';
    const key = locale() + JSON.stringify(options);
    if (!dates.has(key)) dates.set(key, new Intl.DateTimeFormat(locale(), options));
    return dates.get(key).format(parsed);
  }
  function plural(count, forms, variables = {}) {
    const key = locale();
    if (!plurals.has(key)) plurals.set(key, new Intl.PluralRules(key));
    const category = plurals.get(key).select(Number(count));
    const entries = forms?.[language] || forms || {};
    return interpolate(entries[category] ?? entries.other ?? '', { ...variables, count: number(count) });
  }
  window.CopilotI18n = { t, setLanguage, normalize, locale, number, date, plural, get language() { return language; } };
})();
