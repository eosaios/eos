[Setup]
AppId={{EOS-2026-UNIQUE-ID}
AppName=EOS
AppVersion=1.0.0-beta.3
AppPublisher=EOSAIOS
AppPublisherURL=https://github.com/eosaios/eos
AppSupportURL=https://github.com/eosaios/eos/issues
AppUpdatesURL=https://github.com/eosaios/eos/releases/latest
DefaultDirName={autopf}\EOS
DefaultGroupName=EOS
UsePreviousAppDir=yes
AllowNoIcons=yes
OutputDir=output
OutputBaseFilename=eos-setup-1.0.0-beta.3
SetupIconFile=eos.ico
UninstallDisplayIcon={app}\eos.ico
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
PrivilegesRequired=admin
PrivilegesRequiredOverridesAllowed=dialog
ChangesEnvironment=yes
DirExistsWarning=no

[Languages]
Name: "chinesesimplified"; MessagesFile: "compiler:Languages\ChineseSimplified.isl"
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: checkedonce
Name: "addtopath"; Description: "Add to PATH (enables 'eos' command in terminal)"; GroupDescription: "Command Line Access:"; Flags: checkedonce

[Files]
Source: "eos.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "eos.ico"; DestDir: "{app}"; Flags: ignoreversion
Source: "pkg\coreapi\sidecar\core\x86_64-pc-windows-gnu\eos-core.exe"; DestDir: "{app}\core\x86_64-pc-windows-gnu"; Flags: ignoreversion
Source: "pkg\coreapi\sidecar\core\x86_64-pc-windows-gnu\manifest.json"; DestDir: "{app}\core\x86_64-pc-windows-gnu"; Flags: ignoreversion

[Icons]
Name: "{group}\EOS"; Filename: "{app}\eos.exe"; IconFilename: "{app}\eos.ico"
Name: "{group}\{cm:UninstallProgram,EOS}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\EOS"; Filename: "{app}\eos.exe"; Tasks: desktopicon; IconFilename: "{app}\eos.ico"

[Code]

function IsUpgrade: Boolean;
var
  PrevPath: String;
begin
  if RegQueryStringValue(HKLM, 'Software\Microsoft\Windows\CurrentVersion\Uninstall\{EOS-2026-UNIQUE-ID}_is1',
    'InstallLocation', PrevPath) then begin
    Result := DirExists(PrevPath);
  end else begin
    Result := False;
  end;
end;

procedure InitializeWizard;
var
  Page: TOutputMsgWizardPage;
begin
  if IsUpgrade then begin
    Page := CreateOutputMsgPage(wpWelcome,
      'EOS Setup',
      'Upgrading',
      'A previous version of EOS has been detected. This will upgrade your existing installation.');
  end;
end;

const
  EnvRegSubkey = 'SYSTEM\CurrentControlSet\Control\Session Manager\Environment';
  WM_SETTINGCHANGE = $001A;
  SMTO_ABORTIFHUNG = $0002;

function SendMessageTimeout(hWnd: Longint; Msg: Longint; wParam: Longint; lParam: string;
  fuFlags: Longint; uTimeout: Longint; var lpdwResult: Longint): Longint;
  external 'SendMessageTimeoutW@user32.dll stdcall';

function StripPathQuotes(Value: string): string;
begin
  Result := Trim(Value);
  if (Length(Result) >= 2) and (Copy(Result, 1, 1) = '"') and (Copy(Result, Length(Result), 1) = '"') then begin
    Delete(Result, Length(Result), 1);
    Delete(Result, 1, 1);
    Result := Trim(Result);
  end;
end;

function CanonPath(Value: string): string;
begin
  Result := Lowercase(StripPathQuotes(Value));
  while (Length(Result) > 3) and
    ((Copy(Result, Length(Result), 1) = '\') or (Copy(Result, Length(Result), 1) = '/')) do begin
    Delete(Result, Length(Result), 1);
  end;
end;

function NextPathPart(var Source: string): string;
var
  PosSemi: Integer;
begin
  PosSemi := Pos(';', Source);
  if PosSemi = 0 then begin
    Result := Source;
    Source := '';
  end else begin
    Result := Copy(Source, 1, PosSemi - 1);
    Delete(Source, 1, PosSemi);
  end;
end;

procedure AppendPathPart(var Target: string; Part: string);
begin
  Part := Trim(Part);
  if Part = '' then begin
    exit;
  end;
  if Target = '' then begin
    Target := Part;
  end else begin
    Target := Target + ';' + Part;
  end;
end;

function IsLegacyVBPath(Canon: string): Boolean;
begin
  Result :=
    (Canon = CanonPath(ExpandConstant('{autopf}\vb-coding'))) or
    (Canon = CanonPath(ExpandConstant('{autopf}\VB-Coding'))) or
    (Canon = CanonPath(ExpandConstant('{autopf}\VB Coding')));
end;

procedure UpdateMachinePath();
var
  OrigPath: string;
  Remaining: string;
  Part: string;
  CleanPart: string;
  Canon: string;
  TargetCanon: string;
  Seen: string;
  NextPath: string;
  FoundTarget: Boolean;
  Changed: Boolean;
  ResultCode: Longint;
begin
  if not WizardIsTaskSelected('addtopath') then begin
    exit;
  end;

  if not RegQueryStringValue(HKEY_LOCAL_MACHINE,
    EnvRegSubkey, 'Path', OrigPath) then begin
    OrigPath := '';
  end;

  TargetCanon := CanonPath(ExpandConstant('{app}'));
  Remaining := OrigPath;
  Seen := ';';
  NextPath := '';
  FoundTarget := False;
  Changed := False;

  while Remaining <> '' do begin
    Part := NextPathPart(Remaining);
    CleanPart := StripPathQuotes(Part);
    Canon := CanonPath(CleanPart);

    if Canon <> '' then begin
      if Canon = TargetCanon then begin
        if not FoundTarget then begin
          AppendPathPart(NextPath, ExpandConstant('{app}'));
          Seen := Seen + Canon + ';';
          FoundTarget := True;
          if CleanPart <> ExpandConstant('{app}') then begin
            Changed := True;
          end;
        end else begin
          Changed := True;
        end;
      end else if IsLegacyVBPath(Canon) then begin
        Changed := True;
      end else if Pos(';' + Canon + ';', Seen) > 0 then begin
        Changed := True;
      end else begin
        AppendPathPart(NextPath, CleanPart);
        Seen := Seen + Canon + ';';
      end;
    end;
  end;

  if not FoundTarget then begin
    AppendPathPart(NextPath, ExpandConstant('{app}'));
    Changed := True;
  end;

  if NextPath <> OrigPath then begin
    Changed := True;
  end;

  if Changed then begin
    RegWriteExpandStringValue(HKEY_LOCAL_MACHINE, EnvRegSubkey, 'Path', NextPath);
    SendMessageTimeout(HWND_BROADCAST, WM_SETTINGCHANGE, 0, 'Environment',
      SMTO_ABORTIFHUNG, 5000, ResultCode);
  end;
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssPostInstall then begin
    UpdateMachinePath();
  end;
end;

[InstallDelete]
Type: files; Name: "{app}\vb-coding.exe"
Type: files; Name: "{app}\vb.ico"
Type: files; Name: "{autodesktop}\vb-coding.lnk"
Type: filesandordirs; Name: "{autoprograms}\vb-coding"

[UninstallDelete]
Type: filesandordirs; Name: "{app}"
