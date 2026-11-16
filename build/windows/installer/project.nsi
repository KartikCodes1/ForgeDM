Unicode true

####
## ForgeDM installer (NSIS + Modern UI 2).
##
## Built by `wails build -nsis`, which generates wails_tools.nsh next to this
## file and passes the path of ForgeDM.exe as ARG_WAILS_AMD64_BINARY.
## To build by hand after a wails build:
##   makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\ForgeDM.exe project.nsi
##
## ForgeDM installs for the current user (no administrator prompt), into
## %LOCALAPPDATA%\Programs\ForgeDM, like most modern desktop apps.
####

!define INFO_PROJECTNAME    "ForgeDM"
!define INFO_COMPANYNAME    "ForgeDM"
!define INFO_PRODUCTNAME    "ForgeDM"
!define PRODUCT_EXECUTABLE  "ForgeDM.exe"
!define UNINST_KEY_NAME     "ForgeDM"
!ifndef REQUEST_EXECUTION_LEVEL
    !define REQUEST_EXECUTION_LEVEL "user"
!endif
!ifndef WAILS_INSTALL_SCOPE
    !define WAILS_INSTALL_SCOPE "user"
!endif
!define PRODUCT_WEBSITE     "https://github.com/KartikCodes1/ForgeDM"
!define APP_REG_KEY         "Software\ForgeDM"
!define RUN_KEY             "Software\Microsoft\Windows\CurrentVersion\Run"

!include "wails_tools.nsh"
!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "FileFunc.nsh"
!include "Sections.nsh"

VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"
VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "ForgeDM Setup"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

ManifestDPIAware true
SetCompressor /SOLID lzma

Name "ForgeDM"
Caption "ForgeDM ${INFO_PRODUCTVERSION} Setup"
BrandingText "ForgeDM ${INFO_PRODUCTVERSION}"
OutFile "..\..\bin\ForgeDM-Setup-${INFO_PRODUCTVERSION}.exe"
InstallDir "$LOCALAPPDATA\Programs\ForgeDM"
InstallDirRegKey HKCU "${APP_REG_KEY}" "InstallDir"
ShowInstDetails nevershow
ShowUninstDetails nevershow

; ---- look ----
!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
!define MUI_WELCOMEFINISHPAGE_BITMAP "resources\welcome.bmp"
!define MUI_UNWELCOMEFINISHPAGE_BITMAP "resources\welcome.bmp"
!define MUI_HEADERIMAGE
!define MUI_HEADERIMAGE_RIGHT
!define MUI_HEADERIMAGE_BITMAP "resources\header.bmp"
!define MUI_HEADERIMAGE_UNBITMAP "resources\header.bmp"
!define MUI_ABORTWARNING
!define MUI_ABORTWARNING_TEXT "Are you sure you want to stop installing ForgeDM?"
!define MUI_COMPONENTSPAGE_NODESC

; ---- pages ----
!define MUI_WELCOMEPAGE_TITLE "Welcome to ForgeDM"
!define MUI_WELCOMEPAGE_TEXT "ForgeDM downloads files faster by using several connections at once, picks up where it left off after interruptions, catches downloads and videos from your browser, and keeps everything organized.$\r$\n$\r$\nSetup will install ForgeDM ${INFO_PRODUCTVERSION} for your Windows account. You don't need administrator rights.$\r$\n$\r$\nClick Next to continue."
!insertmacro MUI_PAGE_WELCOME

!define MUI_PAGE_HEADER_TEXT "Choose options"
!define MUI_PAGE_HEADER_SUBTEXT "Pick the extras you want. You can change these later in ForgeDM's settings."
!define MUI_COMPONENTSPAGE_TEXT_TOP "Tick the options you want, then click Next."
!define MUI_COMPONENTSPAGE_TEXT_COMPLIST "Options:"
!insertmacro MUI_PAGE_COMPONENTS

!define MUI_PAGE_CUSTOMFUNCTION_LEAVE AskToCloseForgeDM
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES

!define MUI_FINISHPAGE_TITLE "ForgeDM is ready"
!define MUI_FINISHPAGE_TEXT "ForgeDM ${INFO_PRODUCTVERSION} is installed.$\r$\n$\r$\nTo catch downloads from your browser, open ForgeDM, go to Settings, choose Browser integration and click Connect a browser."
!define MUI_FINISHPAGE_RUN "$INSTDIR\${PRODUCT_EXECUTABLE}"
!define MUI_FINISHPAGE_RUN_TEXT "Open ForgeDM now"
!define MUI_FINISHPAGE_LINK "Visit the ForgeDM project page"
!define MUI_FINISHPAGE_LINK_LOCATION "${PRODUCT_WEBSITE}"
!insertmacro MUI_PAGE_FINISH

!define MUI_PAGE_CUSTOMFUNCTION_LEAVE un.AskToCloseForgeDM
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!define MUI_UNFINISHPAGE_NOAUTOCLOSE
!insertmacro MUI_UNPAGE_FINISH

!insertmacro MUI_LANGUAGE "English"

; ---- helpers ----

; $0 = 0 when ForgeDM.exe is running.
!macro IsForgeDMRunning
    nsExec::ExecToStack 'cmd /c tasklist /FI "IMAGENAME eq ${PRODUCT_EXECUTABLE}" /NH | find /I "${PRODUCT_EXECUTABLE}"'
    Pop $0
    Pop $1
!macroend

; Closes ForgeDM (main window and drop target) so its files can be replaced.
; First asks it to save and quit; forces it closed only if it doesn't.
!macro StopForgeDM UN
Function ${UN}StopForgeDM
    !insertmacro IsForgeDMRunning
    ${If} $0 != 0
        Return
    ${EndIf}
    ${If} ${FileExists} "$INSTDIR\${PRODUCT_EXECUTABLE}"
        nsExec::Exec '"$INSTDIR\${PRODUCT_EXECUTABLE}" --quit'
        Pop $1
        StrCpy $2 0
        ${Do}
            Sleep 250
            IntOp $2 $2 + 1
            !insertmacro IsForgeDMRunning
            ${If} $0 != 0
                ${Break}
            ${EndIf}
        ${LoopUntil} $2 >= 24
    ${EndIf}
    !insertmacro IsForgeDMRunning
    ${If} $0 == 0
        nsExec::Exec 'taskkill /F /T /IM ${PRODUCT_EXECUTABLE}'
        Pop $1
        Sleep 600
    ${EndIf}
FunctionEnd

; Asks before closing a running ForgeDM. Abort keeps the wizard on its page.
Function ${UN}AskToCloseForgeDM
    !insertmacro IsForgeDMRunning
    ${If} $0 == 0
        MessageBox MB_OKCANCEL|MB_ICONINFORMATION "ForgeDM is running. Setup will close it to continue.$\r$\n$\r$\nDownloads in progress are saved and pick up where they left off next time." IDOK +2
        Abort
        Call ${UN}StopForgeDM
    ${EndIf}
FunctionEnd
!macroend
!insertmacro StopForgeDM ""
!insertmacro StopForgeDM "un."

; ---- sections ----

Section "ForgeDM (required)" SecApp
    SectionIn RO
    SetShellVarContext current
    Call StopForgeDM

    !insertmacro wails.webview2runtime

    SetOutPath $INSTDIR
    !insertmacro wails.files

    Delete "$DESKTOP\ForgeDM.lnk" ; recreated below if "Desktop shortcut" is ticked
    CreateShortcut "$SMPROGRAMS\ForgeDM.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}" "" "$INSTDIR\${PRODUCT_EXECUTABLE}" 0 SW_SHOWNORMAL "" "Fast download manager"

    WriteRegStr HKCU "${APP_REG_KEY}" "InstallDir" "$INSTDIR"
    WriteRegStr HKCU "${APP_REG_KEY}" "Version" "${INFO_PRODUCTVERSION}"
    ; ForgeDM picks up the startup choice on its next launch (see SecStartup).
    WriteRegDWORD HKCU "${APP_REG_KEY}" "StartWithWindows" 0
    DeleteRegValue HKCU "${RUN_KEY}" "ForgeDM"

    ; Apps & features entry
    WriteUninstaller "$INSTDIR\uninstall.exe"
    WriteRegStr HKCU "${UNINST_KEY}" "DisplayName" "ForgeDM"
    WriteRegStr HKCU "${UNINST_KEY}" "DisplayVersion" "${INFO_PRODUCTVERSION}"
    WriteRegStr HKCU "${UNINST_KEY}" "Publisher" "${INFO_COMPANYNAME}"
    WriteRegStr HKCU "${UNINST_KEY}" "DisplayIcon" "$INSTDIR\${PRODUCT_EXECUTABLE},0"
    WriteRegStr HKCU "${UNINST_KEY}" "InstallLocation" "$INSTDIR"
    WriteRegStr HKCU "${UNINST_KEY}" "URLInfoAbout" "${PRODUCT_WEBSITE}"
    WriteRegStr HKCU "${UNINST_KEY}" "HelpLink" "${PRODUCT_WEBSITE}"
    WriteRegStr HKCU "${UNINST_KEY}" "UninstallString" "$\"$INSTDIR\uninstall.exe$\""
    WriteRegStr HKCU "${UNINST_KEY}" "QuietUninstallString" "$\"$INSTDIR\uninstall.exe$\" /S"
    WriteRegDWORD HKCU "${UNINST_KEY}" "NoModify" 1
    WriteRegDWORD HKCU "${UNINST_KEY}" "NoRepair" 1
    ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
    IntFmt $0 "0x%08X" $0
    WriteRegDWORD HKCU "${UNINST_KEY}" "EstimatedSize" "$0"
SectionEnd

Section "Desktop shortcut" SecDesktop
    SetShellVarContext current
    CreateShortcut "$DESKTOP\ForgeDM.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}" "" "$INSTDIR\${PRODUCT_EXECUTABLE}" 0 SW_SHOWNORMAL "" "Fast download manager"
SectionEnd

Section "Start ForgeDM when Windows starts" SecStartup
    ; ForgeDM applies this choice on its next launch, then keeps its own
    ; "Start with Windows" setting and this Run entry in sync.
    WriteRegDWORD HKCU "${APP_REG_KEY}" "StartWithWindows" 1
    WriteRegStr HKCU "${RUN_KEY}" "ForgeDM" "$\"$INSTDIR\${PRODUCT_EXECUTABLE}$\" --minimized"
SectionEnd

Function .onInit
    !insertmacro wails.checkArchitecture
    SetShellVarContext current
    ; When updating, keep the extras the way they are now.
    ReadRegStr $0 HKCU "${APP_REG_KEY}" "InstallDir"
    ${If} $0 != ""
        ${IfNot} ${FileExists} "$DESKTOP\ForgeDM.lnk"
            !insertmacro UnselectSection ${SecDesktop}
        ${EndIf}
        ReadRegStr $1 HKCU "${RUN_KEY}" "ForgeDM"
        ${If} $1 == ""
            !insertmacro UnselectSection ${SecStartup}
        ${EndIf}
    ${EndIf}
FunctionEnd

; ---- uninstall ----

Function un.onInit
    SetShellVarContext current
FunctionEnd

Section "uninstall"
    SetShellVarContext current
    Call un.StopForgeDM

    Delete "$INSTDIR\${PRODUCT_EXECUTABLE}"
    Delete "$INSTDIR\uninstall.exe"
    RMDir "$INSTDIR"

    Delete "$SMPROGRAMS\ForgeDM.lnk"
    Delete "$DESKTOP\ForgeDM.lnk"

    DeleteRegValue HKCU "${RUN_KEY}" "ForgeDM"
    DeleteRegKey HKCU "${UNINST_KEY}"
    DeleteRegKey HKCU "${APP_REG_KEY}"

    ; Settings, the download list and statistics live in %APPDATA%\ForgeDM.
    ; Downloaded files are never touched.
    IfSilent keepdata
    MessageBox MB_YESNO|MB_ICONQUESTION|MB_DEFBUTTON2 "Also delete your ForgeDM settings and download history?$\r$\n$\r$\nYour downloaded files are kept either way." IDNO keepdata
        RMDir /r "$APPDATA\ForgeDM"
    keepdata:
SectionEnd
