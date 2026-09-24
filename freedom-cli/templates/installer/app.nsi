; Freedom NSIS 安装器模板（freedom build --installer 填充 @...@ 占位符后编译）。
; 占位符：@NAME@ 应用名 / @VERSION@ 版本 / @SRC@ 产物目录（exe + resources）
;        @OUT@ 输出目录 / @EXE@ 可执行文件名。
Unicode true
!include "MUI2.nsh"

!define APPNAME "@NAME@"
!define VERSION "@VERSION@"
!define SRC "@SRC@"
!define OUT "@OUT@"
!define EXE "@EXE@"

Name "${APPNAME}"
OutFile "${OUT}\${APPNAME}-setup-${VERSION}.exe"
InstallDir "$PROGRAMFILES64\${APPNAME}"
InstallDirRegKey HKLM "Software\${APPNAME}" "InstallDir"
RequestExecutionLevel admin

!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "SimpChinese"

Section "Install"
  SetOutPath "$INSTDIR"
  File /r "${SRC}\*.*"
  WriteUninstaller "$INSTDIR\uninstall.exe"
  CreateShortCut "$DESKTOP\${APPNAME}.lnk" "$INSTDIR\${EXE}"
  CreateShortCut "$SMPROGRAMS\${APPNAME}.lnk" "$INSTDIR\${EXE}"
  WriteRegStr HKLM "Software\${APPNAME}" "InstallDir" "$INSTDIR"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APPNAME}" "DisplayName" "${APPNAME}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APPNAME}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APPNAME}" "UninstallString" "$INSTDIR\uninstall.exe"
SectionEnd

Section "Uninstall"
  RMDir /r "$INSTDIR"
  Delete "$DESKTOP\${APPNAME}.lnk"
  Delete "$SMPROGRAMS\${APPNAME}.lnk"
  DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APPNAME}"
  DeleteRegKey HKLM "Software\${APPNAME}"
SectionEnd
