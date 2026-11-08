/* Offline game rank insignia, loaded on demand. Provenance: internal/data/rank-insignia-source.json. */
(()=>{'use strict';
const families=new Set(['combat','trade','explore','cqc','soldier','exobiologist']),navies=new Set(['federation','empire']),cache=new Map(),inflight=new Map(),painted=new WeakMap();let revision=0;
function key(row){
 const n=row?.rankIndex,id=row?.id;
 if(typeof n!=='number'||!Number.isInteger(n)||n<0)return '';
 if(families.has(id)&&n<=13)return id+'/'+n;
 if(navies.has(id)&&n<=14)return id;
 return '';
}
function source(row){return cache.get(key(row))||'';}
function render(row,u,options={}){
 const size=['small','card','hero'].includes(options.size)?options.size:'card',src=source(row),classes='rank-insignia rank-insignia-'+size+(src?'':' is-unknown');
 return '<span class="'+classes+'" aria-hidden="true">'+(src?'<img src="'+src+'" alt="" width="64" height="64" loading="lazy" decoding="async">':u.icon('circle'))+'</span>';
}
async function load(rows,u,C){
 const chosen=new Map();for(const row of Array.isArray(rows)?rows:[]){const id=key(row);if(id&&!cache.has(id)&&chosen.size<16)chosen.set(id,{id:row.id,rankIndex:row.rankIndex});}
 if(!chosen.size)return false;
 const tab=C?.tab,mode=C?.mode,missing=[...chosen].filter(([id])=>!inflight.has(id)),waiting=[...chosen.keys()].filter(id=>inflight.has(id)).map(id=>inflight.get(id));
 if(missing.length){
  const request=Promise.resolve().then(()=>u.call('rank.insignia',{entries:missing.map(([,row])=>row)})).then(response=>{
   let added=false;for(const [id]of missing){const image=response?.images?.[id];if(typeof image==='string'&&image.length<512000&&/^data:image\/png;base64,[A-Za-z0-9+/]+=*$/.test(image)){cache.set(id,image);added=true;}}
   if(added)revision++;
   return added;
  }).finally(()=>{for(const [id]of missing)if(inflight.get(id)===request)inflight.delete(id);});
  for(const [id]of missing)inflight.set(id,request);waiting.push(request);
 }
 await Promise.all(waiting);
 if(C&&C.tab===tab&&C.mode===mode&&!C.layer&&!u.$?.('#page input:focus,#page textarea:focus,#page select:focus')&&(painted.get(C)||0)<revision){painted.set(C,revision);u.renderPage();}
 return [...chosen.keys()].some(id=>cache.has(id));
}
window.RankInsignia=Object.freeze({source,render,load});
})();
