/* Bounded remote system-name combobox. Names and input values are never translated. */
(() => {
  'use strict';
  const singleNames = new Set(['system','sourceSystem','destinationSystem','originSystem','targetSystem']);
  const listNames = new Set(['waypoints','refuelSystems']);
  const inlineIds = new Set(['colony-workspace-origin','reference-market-system']);
  const minimum = 3, delay = 300;
  function fieldMode(node) {
    if (!node || !['INPUT','TEXTAREA'].includes(String(node.tagName).toUpperCase()) || node.disabled || node.readOnly) return '';
    const explicit = node.dataset?.systemSearch;
    if (explicit === 'off') return '';
    if (explicit === 'lines' || explicit === 'single') return explicit;
    const name = node.name || node.getAttribute?.('name') || '';
    if (String(node.tagName).toUpperCase() === 'TEXTAREA') return listNames.has(name) ? 'lines' : '';
    if (singleNames.has(name) || inlineIds.has(node.id)) return 'single';
    if (['source','destination'].includes(name) && node.closest?.('form')?.dataset?.id === 'trade.plan') return 'single';
    const d = node.dataset || {};
    if (d.dealsFilter === 'system' || d.tradeFilter === 'system' || ['sourceSystem','destinationSystem'].includes(d.tradeField)) return 'single';
    return '';
  }
  function lineContext(value, caret, mode = 'lines') {
    value = String(value ?? '');
    if (mode !== 'lines') return {query:value.trim(),start:0,end:value.length,signature:value};
    caret = Math.max(0, Math.min(value.length, Number(caret) || 0));
    const start = value.lastIndexOf('\n', caret - 1) + 1;
    let end = value.indexOf('\n', caret); if (end < 0) end = value.length;
    if (value[end - 1] === '\r') end--;
    const query = value.slice(start,end).trim();
    return {query,start,end,signature:start+':'+query};
  }
  function replaceCurrentLine(value, caret, name, mode = 'lines') {
    const part = lineContext(value,caret,mode), text = String(value ?? '');
    return {value:text.slice(0,part.start)+name+text.slice(part.end),caret:part.start+name.length};
  }
  const countChars = value => [...String(value).replace(/[^\p{L}\p{N}]/gu,'')].length;
  function createController(env = {}) {
    const doc = env.document || document, win = env.window || window;
    const later = env.setTimeout || setTimeout, cancelTimer = env.clearTimeout || clearTimeout;
    let C = {}, u = {}, node = null, popup = null, timer = null, bound = false, composing = false, selecting = false;
    let sequence = 0, fields = 0, epoch = 0, current = null, suggestions = [], selected = -1, status = '', result = null, partSignature = '';
    const sessionID = env.sessionID || win.crypto?.randomUUID?.() || Date.now().toString(36)+'-'+Math.random().toString(36).slice(2,10);
    const ids = new WeakMap(), popupID = 'copilot-system-suggestions';
    const esc = value => u.esc ? u.esc(value) : String(value ?? '').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
    function text(ru,en,vars) {
      const translate = C.t || u.t || win.CopilotI18n?.t;
      const translated = typeof translate === 'function' ? translate(ru,vars) : ru;
      const language = win.CopilotI18n?.language || C.state?.desktop?.language || C.language || 'ru';
      const value = language === 'en' && translated === ru ? en : translated;
      return String(value ?? ru).replace(/\{(\w+)\}/g,(match,key)=>vars && Object.hasOwn(vars,key) ? String(vars[key]) : match);
    }
    function identity(element) { if (!ids.has(element)) ids.set(element,'system-'+sessionID+'-'+(++fields)); return ids.get(element); }
    function activePart(element = node) { return lineContext(element?.value,element?.selectionStart,fieldMode(element)); }
    function connected(element) { return !!element && element.isConnected !== false && doc.activeElement === element; }
    function cancelRequest() {
      if (current && typeof u.call === 'function') {
        const request = current; current = null;
        Promise.resolve(u.call('system-search-cancel',{fieldId:request.fieldId,sequence:request.sequence})).catch(()=>{});
      }
    }
    function close() {
      epoch++; if (timer !== null) cancelTimer(timer); timer = null; cancelRequest();
      if (node) { node.setAttribute?.('aria-expanded','false'); node.removeAttribute?.('aria-activedescendant'); }
      if (popup) popup.hidden = true;
      suggestions = []; selected = -1; status = ''; result = null;
    }
    function mount() {
      if (popup?.isConnected !== false && popup) return;
      popup = doc.createElement('div'); popup.id = popupID; popup.className = 'system-search-popup'; popup.hidden = true;
      popup.setAttribute('data-system-popup',''); doc.body.append(popup);
    }
    function prepare(element) {
      mount(); node = element;
      node.setAttribute('role','combobox'); node.setAttribute('aria-autocomplete','list'); node.setAttribute('aria-haspopup','listbox');
      node.setAttribute('aria-controls',popupID+'-list'); node.setAttribute('aria-expanded','false'); node.setAttribute('autocomplete','off');
      const described = String(node.getAttribute?.('aria-describedby') || '').split(/\s+/).filter(Boolean);
      if (!described.includes(popupID+'-status')) described.push(popupID+'-status');
      node.setAttribute('aria-describedby',described.join(' '));
    }
    function position() {
      if (!popup || popup.hidden || !node?.getBoundingClientRect) return;
      const rect = node.getBoundingClientRect(), width = Math.min(560,Math.max(300,rect.width)), w = win.innerWidth || 1280, h = win.innerHeight || 800;
      const height = Math.min(350, h - 24), below = h - rect.bottom - 8;
      popup.style.width = Math.min(width,w-24)+'px'; popup.style.maxHeight = height+'px';
      popup.style.left = Math.max(12,Math.min(rect.left,w-width-12))+'px';
      popup.style.top = (below >= Math.min(height,230) ? rect.bottom + 6 : Math.max(12,rect.top-height-6))+'px';
    }
    function message() {
      if (status === 'loading') return text('Ищем системы…','Searching systems…');
      if (status === 'too-short') return text('Введите не менее {count} символов','Enter at least {count} characters',{count:minimum});
      if (status === 'rate-limited') return text('Сервис временно ограничил запросы. Повторите через {count} с.','The service has temporarily limited requests. Try again in {count}s.',{count:Math.max(1,Math.ceil(Number(result?.retryAfterMs || 30000)/1000))});
      if (status === 'disabled') return text('Сетевой поиск отключён. Название можно ввести вручную.','Online search is disabled. You can enter the name manually.');
      if (status === 'error') return text('Не удалось получить подсказки. Можно повторить поиск или ввести название вручную.','Suggestions could not be loaded. Retry or enter the name manually.');
      if (status === 'empty') return text('Подходящая система не найдена. Проверьте написание или уточните название.','No matching system was found. Check the spelling or refine the name.');
      return text('Найдено вариантов: {count}','Suggestions found: {count}',{count:suggestions.length});
    }
    function render() {
      if (!connected(node)) { close(); return; }
      mount(); popup.hidden = false; node.setAttribute('aria-expanded','true');
      const retry = status === 'error' || status === 'rate-limited';
      popup.innerHTML = `<div class="system-search-status ${status === 'error' ? 'is-error' : ''}" id="${popupID}-status" role="status" aria-live="polite">${esc(message())}${retry?`<button type="button" data-system-retry>${esc(text('Повторить','Retry'))}</button>`:''}</div><div class="system-search-list" id="${popupID}-list" role="listbox" aria-label="${esc(text('Звёздные системы','Star systems'))}">${suggestions.map((item,i)=>`<div class="system-search-option ${i===selected?'is-selected':''}" role="option" aria-selected="${i===selected}" id="${popupID}-option-${i}" data-system-option="${i}"><strong>${esc(item.name)}</strong>${item.match==='correction'?`<span>${esc(text('Возможно, вы имели в виду','Did you mean'))}</span>`:''}</div>`).join('')}</div><div class="system-search-footer"><span>${esc(result?.limited?text('Выдача API ограничена. Уточните название.','The API result list is limited. Refine the name.'):fieldMode(node)==='lines'?text('Выбор заменит только текущую строку.','Selection replaces only the current line.'):text('Enter — выбрать · Esc — закрыть','Enter to select · Esc to close'))}</span><span>Spansh</span></div>`;
      if (selected >= 0) node.setAttribute('aria-activedescendant',popupID+'-option-'+selected); else node.removeAttribute('aria-activedescendant');
      position(); if (selected >= 0) popup.querySelector?.('[data-system-option="'+selected+'"]')?.scrollIntoView?.({block:'nearest'});
    }
    function schedule(element, immediate = false) {
      if (!fieldMode(element)) return false;
      if (node !== element) close();
      prepare(element); epoch++; const token = epoch;
      if (timer !== null) cancelTimer(timer); timer = null; cancelRequest();
      suggestions = []; selected = -1; result = null;
      const part = activePart(element);
      partSignature = part.signature;
      if (countChars(part.query) < minimum) { status = 'too-short'; render(); return true; }
      if (composing) { close(); return true; }
      status = 'loading'; render();
      timer = later(async()=>{
        timer = null;
        if (token !== epoch) return;
        if (!connected(element)) { close(); return; }
        if (activePart(element).signature !== part.signature) return;
        const request = {query:part.query,fieldId:identity(element),sequence:++sequence,limit:20}; current = request;
        try {
          const response = await u.call('system-search',request);
          if (token !== epoch) return;
          if (!connected(element)) { close(); return; }
          if (activePart(element).signature !== part.signature || response?.sequence !== request.sequence || response?.fieldId !== request.fieldId) return;
          current = null; result = response;
          if (response.status === 'cancelled') { close(); return; }
          status = ['ready','empty','error','rate-limited','disabled','too-short'].includes(response.status) ? response.status : 'error';
          suggestions = status === 'ready' && Array.isArray(response.suggestions) ? response.suggestions.filter(item=>item && typeof item.name === 'string' && item.name.trim()).slice(0,20) : [];
          if (status === 'ready' && !suggestions.length) status = 'empty';
          render();
        } catch (_error) {
          if (token !== epoch) return;
          if (!connected(element)) { close(); return; }
          current = null; status = 'error'; suggestions = []; render();
        }
      },immediate?0:delay);
      return true;
    }
    function choose(index) {
      const item = suggestions[index], input = node;
      if (!item || !connected(input)) return false;
      const replacement = replaceCurrentLine(input.value,input.selectionStart,item.name,fieldMode(input));
      close(); selecting = true;
      try {
        input.value = replacement.value; input.focus?.({preventScroll:true}); input.setSelectionRange?.(replacement.caret,replacement.caret);
        const EventType = env.Event || win.Event || Event;
        input.dispatchEvent(new EventType('input',{bubbles:true})); input.dispatchEvent(new EventType('change',{bubbles:true}));
      } finally { selecting = false; }
      return true;
    }
    function keydown(event) {
      if (event.target !== node || popup?.hidden || !fieldMode(node) || composing || event.isComposing) return false;
      if (event.key === 'Escape') { event.preventDefault(); event.stopImmediatePropagation?.(); close(); return true; }
      if (event.key === 'Tab') { close(); return false; }
      if (event.key === 'ArrowDown' && ['error','rate-limited'].includes(status)) { event.preventDefault(); event.stopImmediatePropagation?.(); schedule(node,true); return true; }
      if (!suggestions.length) return false;
      let next = selected;
      if (event.key === 'ArrowDown') next = (selected + 1) % suggestions.length;
      else if (event.key === 'ArrowUp') next = (selected - 1 + suggestions.length) % suggestions.length;
      else if (event.key === 'Home' && selected >= 0) next = 0;
      else if (event.key === 'End' && selected >= 0) next = suggestions.length - 1;
      else if (event.key === 'Enter' && selected >= 0) { event.preventDefault(); event.stopImmediatePropagation?.(); return choose(selected); }
      else return false;
      event.preventDefault(); event.stopImmediatePropagation?.(); selected = next; render(); return true;
    }
    function pointerdown(event) {
      const option = event.target?.closest?.('[data-system-option]');
      if (option && popup?.contains?.(option)) { event.preventDefault(); event.stopImmediatePropagation?.(); return choose(Number(option.dataset.systemOption)); }
      const retry = event.target?.closest?.('[data-system-retry]');
      if (retry && popup?.contains?.(retry)) { event.preventDefault(); event.stopImmediatePropagation?.(); schedule(node,true); return true; }
      if (event.target !== node && !popup?.contains?.(event.target)) close();
      return false;
    }
    function input(event) { if (selecting || event.isComposing || composing) return false; return schedule(event.target); }
    function focus(event) { if (!fieldMode(event.target)) { close(); return false; }; return schedule(event.target); }
    function caret(event) {
      if (event.target === node && fieldMode(node) === 'lines' && activePart(node).signature !== partSignature) {
        if (!['ArrowDown','ArrowUp','Home','End','Enter','Escape','Tab'].includes(event.key || '')) schedule(node);
      }
    }
    function bind() {
      if (bound) return; bound = true;
      doc.addEventListener('focusin',focus,true); doc.addEventListener('input',input,true);
      doc.addEventListener('keydown',keydown,true); doc.addEventListener('pointerdown',pointerdown,true);
      doc.addEventListener('compositionstart',event=>{if(fieldMode(event.target)){composing=true;close();}},true);
      doc.addEventListener('compositionend',event=>{if(fieldMode(event.target)){composing=false;schedule(event.target);}},true);
      doc.addEventListener('keyup',caret,true); doc.addEventListener('click',caret,true);
      doc.addEventListener('focusout',event=>{if(event.target===node)later(()=>{if(doc.activeElement!==node)close();},0);},true);
      doc.addEventListener('scroll',()=>{if(popup&&!popup.hidden)position();},true);
      win.addEventListener?.('resize',position);
    }
    return {update(nextC,nextU){C=nextC||{};u=nextU||{};if(node&&!connected(node))close();},bind,close,schedule,choose,keydown,pointerdown,input,focus,
      inspect:()=>({node,popup,status,selected,suggestions:[...suggestions],current,epoch})};
  }
  let controller;
  window.SystemSearchUI = {fieldMode,lineContext,replaceCurrentLine,createController,
    init(C,u){if(!controller)controller=createController();controller.update(C,u);controller.bind();},
    close(){controller?.close();}};
})();
