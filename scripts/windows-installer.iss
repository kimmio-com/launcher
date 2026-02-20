#define MyAppName "Kimmio Launcher"
#ifndef AppVersion
  #define AppVersion "1.0.0"
#endif
#ifndef AppExeName
  #define AppExeName "KimmioLauncher.exe"
#endif
#ifndef SourceDir
  #define SourceDir "dist\\windows\\Kimmio Launcher"
#endif
#ifndef OutputDir
  #define OutputDir "dist"
#endif

[Setup]
AppId={{6DDA0477-D2BC-4274-95E4-6D5CFEC2CB6A}
AppName={#MyAppName}
AppVersion={#AppVersion}
AppPublisher=Kimmio
DefaultDirName={autopf}\{#MyAppName}
DefaultGroupName={#MyAppName}
DisableProgramGroupPage=yes
LicenseFile={#SourceDir}\LICENSE.txt
OutputDir={#OutputDir}
OutputBaseFilename=Kimmio-Launcher-Setup-windows-amd64
Compression=lzma
SolidCompression=yes
WizardStyle=modern
PrivilegesRequired=admin
ArchitecturesInstallIn64BitMode=x64

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "Create a desktop icon"; GroupDescription: "Additional icons:"; Flags: unchecked

[Files]
Source: "{#SourceDir}\\{#AppExeName}"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceDir}\\AppIcon.ico"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceDir}\\README.txt"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceDir}\\LICENSE.txt"; DestDir: "{app}"; Flags: ignoreversion skipifsourcedoesntexist

[Icons]
Name: "{autoprograms}\\{#MyAppName}"; Filename: "{app}\\{#AppExeName}"; IconFilename: "{app}\\AppIcon.ico"
Name: "{autodesktop}\\{#MyAppName}"; Filename: "{app}\\{#AppExeName}"; IconFilename: "{app}\\AppIcon.ico"; Tasks: desktopicon

[Run]
Filename: "{app}\\{#AppExeName}"; Description: "Launch {#MyAppName}"; Flags: nowait postinstall skipifsilent
