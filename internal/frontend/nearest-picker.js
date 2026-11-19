(() => {
  'use strict';

  let current, popup;
  const listId = 'nearest-picker-options';
  const trigger = field => field?.closest('.near-picker')?.querySelector('[data-near-picker]');
  const normalize = value => String(value || '').toLocaleLowerCase().normalize('NFD').replace(/\p{M}/gu,'');
  function match(query, choices) {
    const words = normalize(query).trim().split(/\s+/).filter(Boolean);
    return choices.filter(choice => words.every(word => choice.search.includes(word)));
  }

  function update(field) {
    const button = trigger(field);
    if (!button) return;
    const label = field.selectedOptions[0]?.label || '';
    button.querySelector('span').textContent = label;
    button.setAttribute('aria-label',[button.dataset.nearLabel,label].filter(Boolean).join(' · '));
  }

  function close(restoreFocus=false) {
    if (!current) return;
    const button = current.button;
    current = null;
    popup.hidden = true;
    button.setAttribute('aria-expanded','false');
    button.removeAttribute('aria-controls');
    if (restoreFocus && button.isConnected) button.focus({preventScroll:true});
  }

  function position() {
    if (!current) return;
    const {button} = current;
    if (!button.isConnected) return close();
    const anchor = button.getBoundingClientRect();
    const gutter = 8, width = Math.min(innerWidth-2*gutter,Math.max(220,anchor.width));
    if (anchor.bottom<gutter || anchor.top>innerHeight-gutter) return close();
    const below = innerHeight-anchor.bottom-2*gutter, above = anchor.top-2*gutter;
    const openBelow = below>=290 || below>=above;
    popup.style.width = width+'px';
    popup.style.maxHeight = Math.max(72,Math.min(342,openBelow?below:above))+'px';
    popup.style.left = Math.max(gutter,Math.min(anchor.left,innerWidth-width-gutter))+'px';
    popup.style.top = openBelow ? anchor.bottom+gutter+'px' : 'auto';
    popup.style.bottom = openBelow ? 'auto' : innerHeight-anchor.top+gutter+'px';
  }

  function activeOption(index, scroll) {
    const {list,search,visible} = current;
    const previous = list.querySelector('.is-selected');
    previous?.classList.remove('is-selected');
    previous?.setAttribute('aria-selected','false');
    search.removeAttribute('aria-activedescendant');
    current.index = visible.length ? Math.max(0,Math.min(visible.length-1,index)) : -1;
    if (current.index<0) return;
    const id = listId+'-'+current.index, next = document.getElementById(id);
    search.setAttribute('aria-activedescendant',id);
    next.classList.add('is-selected');
    next.setAttribute('aria-selected','true');
    if (scroll) next.scrollIntoView({block:'nearest'});
  }

  function draw(first=false) {
    const {list,search,choices,field,u} = current;
    const visible = current.visible = match(search.value,choices);
    let group = '';
    list.replaceChildren();
    for (const [i,item] of visible.entries()) {
      if (item.group && group!==item.group) {
        const heading = document.createElement('div');
        heading.className = 'near-picker-group';
        heading.setAttribute('role','presentation');
        heading.textContent = item.group;
        list.append(heading);
      }
      group = item.group;
      const option = document.createElement('div');
      option.id = listId+'-'+i;
      option.className = 'system-search-option near-picker-option';
      option.setAttribute('role','option');
      option.setAttribute('aria-selected','false');
      if (item.group) option.setAttribute('aria-label',item.group+' · '+item.label);
      option.dataset.nearChoice = i;
      const label = document.createElement('strong');
      label.textContent = item.label;
      option.append(label);
      list.append(option);
    }
    current.empty.hidden = visible.length>0;
    current.empty.textContent = visible.length ? '' : u.t('Ничего не найдено.');
    list.scrollTop = 0;
    activeOption(first ? visible.findIndex(item=>item.value===field.value) : 0,first);
  }

  function choose(index) {
    const item = current.visible[index], {field} = current;
    if (!item) return;
    close(true);
    if (field.value===item.value) return;
    field.value = item.value;
    update(field);
    field.dispatchEvent(new Event('change',{bubbles:true}));
  }

  function keydown(event) {
    if (!current || event.isComposing) return;
    if (event.key==='Tab') {
      close(true);
      return;
    }
    const {visible,index} = current;
    switch (event.key) {
      case 'Escape': close(true); break;
      case 'Enter': choose(index); break;
      case 'ArrowUp': activeOption((index+visible.length-1)%visible.length,true); break;
      case 'ArrowDown': activeOption((index+1)%visible.length,true); break;
      case 'PageUp': activeOption(index-8,true); break;
      case 'PageDown': activeOption(index+8,true); break;
      default: return;
    }
    event.preventDefault();
    event.stopPropagation();
  }

  function mount() {
    if (popup) return;
    popup = document.createElement('div');
    popup.className = 'system-search-popup near-picker-popup';
    popup.hidden = true;
    document.body.append(popup);
    popup.addEventListener('keydown',keydown);
    popup.addEventListener('pointerdown',event=>{
      if (event.target.closest('[data-near-choice]')) event.preventDefault();
    });
    popup.addEventListener('click',event=>{
      const item = event.target.closest('[data-near-choice]');
      if (current && item) choose(Number(item.dataset.nearChoice));
    });
    document.addEventListener('pointerdown',event=>{
      if (current && !current.button.contains(event.target) && !popup.contains(event.target)) close();
    },true);
    document.addEventListener('focusin',event=>{
      if (current && event.target!==current.button && !popup.contains(event.target)) close();
    },true);
    document.addEventListener('scroll',event=>{if (!popup.contains(event.target)) position();},true);
    window.addEventListener('resize',position);
  }

  function toggle(button,u) {
    if (current?.button===button) return close(true);
    close();
    const field = button.previousElementSibling;
    if (!field || field.disabled) return;
    mount();
    const search = document.createElement('input'), list = document.createElement('div'), empty = document.createElement('p');
    search.type = 'search';
    search.placeholder = u.t('Поиск по списку');
    search.setAttribute('aria-label',u.t('Поиск по списку'));
    search.setAttribute('role','combobox');
    search.setAttribute('aria-expanded','true');
    search.setAttribute('aria-autocomplete','list');
    search.setAttribute('aria-controls',listId);
    search.autocomplete = 'off';
    search.addEventListener('input',()=>draw());
    search.addEventListener('compositionend',()=>draw());
    list.className = 'system-search-list';
    list.setAttribute('role','listbox');
    list.setAttribute('aria-label',button.dataset.nearLabel || u.t('Варианты'));
    list.id = listId;
    empty.className = 'near-picker-empty';
    empty.setAttribute('role','status');
    const choices = Array.from(field.options,option=>{
      const group = option.parentElement.tagName==='OPTGROUP' ? option.parentElement.label : '';
      return {label:option.label,value:option.value,group,search:normalize([option.label,option.value,group,option.dataset.nearAlias].join(' '))};
    });
    current = {button,field,choices,list,search,empty,u};
    popup.replaceChildren(search,list,empty);
    popup.hidden = false;
    button.setAttribute('aria-expanded','true');
    button.setAttribute('aria-controls',listId);
    position();
    if (!current) return;
    draw(true);
    search.focus({preventScroll:true});
  }

  window.NearestPicker = {toggle,close,update,trigger,match,normalize,
    sync(host){if (current && !host.contains(current.button)) close();}};
})();
