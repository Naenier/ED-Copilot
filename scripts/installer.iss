#ifndef ApplicationVersion
  #error ApplicationVersion is required
#endif
#ifndef PayloadDir
  #error PayloadDir is required
#endif
#ifndef OutputDir
  #error OutputDir is required
#endif
#ifndef InstallerFileName
  #define InstallerFileName "ED-Copilot-" + ApplicationVersion + "-Setup"
#endif
#ifndef WebViewBootstrapper
  #error A verified Microsoft WebView2 bootstrapper is required
#endif

[Setup]
#ifdef TestAppId
AppId=EDCopilot.InstallerSmoke.{#TestAppId}
AppMutex=Local\EDCopilot.InstallerSmoke.{#TestAppId}
#else
AppId=EDCopilot.Desktop
AppMutex=Local\EDCopilot.Desktop.Session
#endif
AppName=ED Copilot
AppVersion={#ApplicationVersion}
AppVerName=ED Copilot {#ApplicationVersion}
AppPublisher=Naenier
AppPublisherURL=https://github.com/Naenier/ED-Copilot
AppSupportURL=https://github.com/Naenier/ED-Copilot/issues
AppUpdatesURL=https://github.com/Naenier/ED-Copilot/releases
VersionInfoVersion={#ApplicationVersion}
VersionInfoDescription=ED Copilot — установка
DefaultDirName={localappdata}\Programs\ED Copilot
DefaultGroupName=ED Copilot
DisableProgramGroupPage=yes
DisableDirPage=no
DisableWelcomePage=no
UsePreviousAppDir=yes
UsePreviousTasks=yes
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0
OutputDir={#OutputDir}
OutputBaseFilename={#InstallerFileName}
SourceDir={#PayloadDir}
SetupIconFile={#SourcePath}..\internal\data\copilot.ico
UninstallDisplayIcon={app}\ED Copilot.exe
UninstallDisplayName=ED Copilot
UninstallFilesDir={app}\uninstall
CloseApplications=no
RestartApplications=no
RestartIfNeededByRun=no
ChangesAssociations=no
ChangesEnvironment=no
SetupLogging=yes
Compression=lzma2/max
SolidCompression=yes
WizardStyle=modern dark includetitlebar
WizardSizePercent=112
Uninstallable=yes
CreateUninstallRegKey=yes

[Languages]
Name: "russian"; MessagesFile: "compiler:Languages\Russian.isl"
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "startmenuicon"; Description: "{cm:StartMenuShortcut}"; GroupDescription: "{cm:ShortcutGroup}"
Name: "desktopicon"; Description: "{cm:DesktopShortcut}"; GroupDescription: "{cm:ShortcutGroup}"; Flags: unchecked

[CustomMessages]
english.ShortcutGroup=Shortcuts:
english.StartMenuShortcut=Create a Start Menu shortcut
english.DesktopShortcut=Create a desktop shortcut
english.LaunchCopilot=Launch ED Copilot
russian.ShortcutGroup=Ярлыки:
russian.StartMenuShortcut=Создать ярлык в меню «Пуск»
russian.DesktopShortcut=Создать ярлык на рабочем столе
russian.LaunchCopilot=Запустить ED Copilot

[Files]
Source: "ED Copilot.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "README.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "README.ru.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "docs\*"; DestDir: "{app}\docs"; Flags: ignoreversion recursesubdirs createallsubdirs
Source: "lib\*"; DestDir: "{app}\lib"; Flags: ignoreversion recursesubdirs createallsubdirs
Source: "{#WebViewBootstrapper}"; Flags: dontcopy

[Icons]
#ifdef TestAppId
Name: "{app}\smoke-start-menu\ED Copilot"; Filename: "{app}\ED Copilot.exe"; WorkingDir: "{app}"; Tasks: startmenuicon
Name: "{app}\smoke-desktop\ED Copilot"; Filename: "{app}\ED Copilot.exe"; WorkingDir: "{app}"; Tasks: desktopicon
#else
Name: "{autoprograms}\ED Copilot"; Filename: "{app}\ED Copilot.exe"; WorkingDir: "{app}"; Tasks: startmenuicon
Name: "{autodesktop}\ED Copilot"; Filename: "{app}\ED Copilot.exe"; WorkingDir: "{app}"; Tasks: desktopicon
#endif

[InstallDelete]
Type: files; Name: "{app}\smoke-headless.json"
Type: files; Name: "{app}\smoke-native.json"
Type: files; Name: "{app}\smoke-stress.json"
Type: files; Name: "{app}\smoke-installer.json"
#ifdef TestAppId
Type: files; Name: "{app}\smoke-start-menu\ED Copilot.lnk"; Tasks: not startmenuicon
Type: files; Name: "{app}\smoke-desktop\ED Copilot.lnk"; Tasks: not desktopicon
#else
Type: files; Name: "{autoprograms}\ED Copilot.lnk"; Tasks: not startmenuicon
Type: files; Name: "{autodesktop}\ED Copilot.lnk"; Tasks: not desktopicon
#endif

[Run]
Filename: "{app}\ED Copilot.exe"; Description: "{cm:LaunchCopilot}"; Flags: nowait postinstall skipifsilent

[Code]
const
  WebViewRegistry = 'Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}';

var
  RuntimePage: TInputOptionWizardPage;
  ShortcutMigrationApplied: Boolean;

function GetCurrentProcessId: LongWord;
  external 'GetCurrentProcessId@kernel32.dll stdcall';
function ProcessIdToSessionId(ProcessId: LongWord; var SessionId: LongWord): Boolean;
  external 'ProcessIdToSessionId@kernel32.dll stdcall';

function RuntimeVersionExists(const RootKey: Integer): Boolean;
var
  Text: String;
  PackedVersion: Int64;
begin
  Result := RegQueryStringValue(RootKey, WebViewRegistry, 'pv', Text);
  if Result then
    Result := StrToVersion(Text, PackedVersion) and (PackedVersion > 0);
end;

function WebViewInstalled: Boolean;
begin
#ifdef TestMissingRuntime
  Result := False;
#else
  Result := RuntimeVersionExists(HKCU32) or RuntimeVersionExists(HKLM32);
#endif
end;

function HasParameter(const Name: String): Boolean;
var
  I: Integer;
begin
  Result := False;
  for I := 1 to ParamCount do
    if CompareText(ParamStr(I), '/' + Name) = 0 then
    begin
      Result := True;
      Exit;
    end;
end;

function ApplicationRunning: Boolean;
var
  Locator, Services, Processes: Variant;
  SessionId: LongWord;
  Query: String;
begin
  Result := False;
#ifndef TestAppId
  { The mutex covers this release and later versions. This compatibility
    query also detects the previous portable release which had no mutex. }
  try
    Locator := CreateOleObject('WbemScripting.SWbemLocator');
    Services := Locator.ConnectServer('', 'root\CIMV2');
    Query := 'SELECT ProcessId FROM Win32_Process WHERE Name = ''ED Copilot.exe''';
    if ProcessIdToSessionId(GetCurrentProcessId, SessionId) then
      Query := Query + ' AND SessionId = ' + IntToStr(SessionId);
    Processes := Services.ExecQuery(Query);
    Result := Processes.Count > 0;
  except
    { Never terminate a process or change app data to work around detection. }
    Log('Compatibility process query unavailable; mutex and file locking remain active.');
  end;
#endif
end;

function ShortcutTasksOverridden: Boolean;
var
  I: Integer;
  Parameter: String;
begin
  Result := HasParameter('NOICONS');
  for I := 1 to ParamCount do
  begin
    Parameter := UpperCase(ParamStr(I));
    if (Copy(Parameter, 1, 7) = '/TASKS=') or
       (Copy(Parameter, 1, 12) = '/MERGETASKS=') then
      Result := True;
  end;
end;

procedure RestoreLegacyShortcutChoice;
var
  PreviousStartMenuLink: String;
begin
  if ShortcutMigrationApplied then
    Exit;
  ShortcutMigrationApplied := True;
  { Releases before independent shortcut tasks always created a Start Menu
    link. Preserve that existing choice once, without overriding an explicit
    task command line or any choices saved by this release. }
  if (GetPreviousData('ShortcutTasksV2', '') = '') and
     not ShortcutTasksOverridden then
  begin
#ifdef TestAppId
    PreviousStartMenuLink := ExpandConstant('{app}\smoke-start-menu\ED Copilot.lnk');
#else
    PreviousStartMenuLink := ExpandConstant('{autoprograms}\ED Copilot.lnk');
#endif
    if FileExists(PreviousStartMenuLink) then
      WizardSelectTasks('startmenuicon');
  end;
end;

procedure InitializeWizard;
begin
  RuntimePage := CreateInputOptionPage(wpSelectDir,
    'Компонент интерфейса', 'Microsoft Edge WebView2 Runtime',
    'Для интерфейса ED Copilot нужен компонент Microsoft Edge WebView2 Runtime. Если его нет, установщик Microsoft загрузит и установит его. Потребуется интернет.',
    False, False);
  RuntimePage.Add('Установить Microsoft Edge WebView2 Runtime');
  RuntimePage.Values[0] := True;
end;

procedure CurPageChanged(CurPageID: Integer);
begin
  if CurPageID = wpSelectTasks then
    RestoreLegacyShortcutChoice;
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssInstall then
    RestoreLegacyShortcutChoice;
end;

procedure RegisterPreviousData(PreviousDataKey: Integer);
begin
  SetPreviousData(PreviousDataKey, 'ShortcutTasksV2', '2');
end;

function ShouldSkipPage(PageID: Integer): Boolean;
begin
  Result := (PageID = RuntimePage.ID) and WebViewInstalled;
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  ExitCode: Integer;
  InstallRuntime: Boolean;
begin
  Result := '';
  if ApplicationRunning then
  begin
    Result := 'ED Copilot работает. Завершите его через значок в области уведомлений Windows → «Выход», затем повторите установку.';
    Exit;
  end;
  if WebViewInstalled then
    Exit;
  if WizardSilent then
    InstallRuntime := HasParameter('INSTALLWEBVIEW2')
  else
    InstallRuntime := RuntimePage.Values[0];
  if not InstallRuntime then
  begin
    Result := 'Не установлен Microsoft Edge WebView2 Runtime. Установите компонент с сайта Microsoft или разрешите его установку на предыдущем экране. Для автоматической установки используется параметр /INSTALLWEBVIEW2.';
    Exit;
  end;
#ifdef TestAppId
  { Tests exercise the refusal path without changing Windows components. }
  Result := 'Изолированная проверка установщика не устанавливает WebView2.';
  Exit;
#else
  ExtractTemporaryFile('MicrosoftEdgeWebview2Setup.exe');
  if not Exec(ExpandConstant('{tmp}\MicrosoftEdgeWebview2Setup.exe'), '/silent /install', '', SW_HIDE, ewWaitUntilTerminated, ExitCode) then
  begin
    Result := 'Не удалось запустить установщик Microsoft WebView2. Установите компонент с официального сайта Microsoft и повторите установку ED Copilot.';
    Exit;
  end;
  if ExitCode = 3010 then
    NeedsRestart := True;
  if ((ExitCode <> 0) and (ExitCode <> 3010)) or not WebViewInstalled then
    Result := 'Microsoft WebView2 не был установлен. Проверьте интернет и повторите установку компонента Microsoft; файлы ED Copilot ещё не изменены.';
#endif
end;

function InitializeUninstall: Boolean;
begin
  Result := not ApplicationRunning;
  if not Result then
    MsgBox('Завершите ED Copilot через значок в области уведомлений Windows → «Выход», затем повторите удаление.', mbError, MB_OK);
end;
