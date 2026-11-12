/* Quiet background updates; installation is deferred until a full application exit. */
(()=>{'use strict';
const text=(C,ru,en)=>(C.state?.settings?.language||C.state?.app?.language||'en')==='en'?en:ru;
function renderSettings(C,u){
 const x=C.state?.updates||{},enabled=C.state?.settings?.autoUpdateEnabled!==false;
 const label=text(C,'Обновления приложения','Application updates');
 const recoveryStatus={recovery:text(C,'Предыдущая установка обновления не подтверждена.','The previous update could not be confirmed.'),restartRequired:text(C,'Для завершения обновления перезапустите Windows.','Restart Windows to finish updating.')}[x.status];
 const status=recoveryStatus||{disabled:text(C,'Автообновление выключено','Automatic updates are off'),idle:text(C,'Проверяем новые стабильные релизы в фоне.','Stable releases are checked in the background.'),checking:text(C,'Проверяем обновления…','Checking for updates…'),downloading:text(C,'Загружаем обновление в фоне…','Downloading an update in the background…'),staged:text(C,'Обновление готово. Установится при полном завершении приложения.','Update ready. It will install when you fully exit the application.'),ready:text(C,'Обновление готово. Установится при полном завершении приложения.','Update ready. It will install when you fully exit the application.'),current:text(C,'Установлена актуальная версия.','Your application is up to date.'),error:text(C,'Обновление пока недоступно. Повторим позже.','Updates are temporarily unavailable. We will retry later.'),portable:text(C,'Для тихого обновления установите приложение через установщик.','Install the application with Setup to enable quiet installation.')}[x.status]||text(C,'Проверяем новые стабильные релизы в фоне.','Stable releases are checked in the background.');
 const busy=['checking','downloading'].includes(x.status);
 const recovery=x.recoveryPath?u.btn(text(C,'Копировать путь к прежней версии','Copy previous version path'),'updates-recovery-copy','copy','quiet'):'';
 return `<section class="settings-group updates-settings" data-updates-settings><h3>${u.icon('refresh-cw')}${u.esc(label)}</h3><label class="check-label updates-toggle"><input type="checkbox" id="auto-update-enabled" ${enabled?'checked':''}><span>${u.esc(text(C,'Обновлять автоматически','Update automatically'))}</span></label><p class="info-note" role="status">${u.esc(enabled?status:text(C,'Автообновление выключено','Automatic updates are off'))}</p><dl class="updates-facts"><div><dt>${u.esc(text(C,'Установлено','Installed'))}</dt><dd>${u.esc(C.state?.app?.version||x.version||'—')}</dd></div>${x.availableVersion?`<div><dt>${u.esc(text(C,'Новая версия','New version'))}</dt><dd>${u.esc(x.availableVersion)}</dd></div>`:''}${x.lastChecked?`<div><dt>${u.esc(text(C,'Проверено','Last checked'))}</dt><dd>${u.esc(u.date?u.date(x.lastChecked):x.lastChecked)}</dd></div>`:''}</dl><p class="info-note">${u.esc(text(C,'Загрузка проходит без окон. Установка начинается после «Файл → Завершить ED Copilot», при следующем запуске появится список изменений. Закрытие в трей оставляет приложение работать.','Downloads run without dialogs. Installation starts after File → Exit ED Copilot; the next launch shows what changed. Closing to the tray keeps the application running.'))}</p>${u.btn(text(C,'Проверить сейчас','Check now'),'updates-check','refresh-cw','quiet',busy||!enabled?'disabled':'')}${recovery}${x.error?`<details class="updates-error"><summary>${u.esc(text(C,'Сведения о проверке','Check details'))}</summary><p>${u.esc(x.error)}</p></details>`:''}</section>`;
}
async function handleChange(event,C,u){
 if(event.target?.id!=='auto-update-enabled')return false;
 const input=event.target,previous=C.state?.settings?.autoUpdateEnabled!==false;input.disabled=true;
 try{await u.call('settings',{...u.context(),values:{autoUpdateEnabled:input.checked}});await u.loadView();}
 catch(error){input.checked=previous;throw error;}finally{input.disabled=false;}
 return true;
}
async function handleClick(action,button,C,u){
 if(action==='updates-recovery-copy'){
  const path=C.state?.updates?.recoveryPath;
  if(path){await u.call('native',{action:'copy',text:path});u.toast(text(C,'Путь скопирован','Path copied'));}
  return true;
 }
 if(action==='updates-check'){button.disabled=true;try{await u.call('updates-check');await u.loadView();}finally{button.disabled=false;}return true;}
 if(action==='updates-dismiss'){await u.call('updates-dismiss');u.closeLayer();return true;}
 return false;
}
function showNotice(C,u){
 const notice=C.state?.updates?.notice;
 if(window.COPILOT_SMOKE||C.layer||!notice?.version||C.updateNoticeShown===notice.version)return false;
 C.updateNoticeShown=notice.version;
 u.dialog(text(C,'ED Copilot обновлён до ','ED Copilot updated to ')+notice.version,`<div class="updates-changelog"><p>${u.esc(text(C,'Обновление установлено. Ваш профиль и планы сохранены.','The update is installed. Your profile and plans have been preserved.'))}</p><pre>${u.esc(notice.body||text(C,'Улучшения и исправления приложения.','Application improvements and fixes.'))}</pre></div>`,u.btn(text(C,'Продолжить','Continue'),'updates-dismiss','','primary'));
 return true;
}
window.UpdatesUI={renderSettings,handleChange,handleClick,showNotice};
})();
