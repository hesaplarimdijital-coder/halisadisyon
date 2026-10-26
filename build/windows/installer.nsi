; Halis Adisyon · Türkçe kurulum sihirbazı (NSIS 3)
; makensis -DSTAGE=<klasör> -DVERSION=2.3.0 -DOUTFILE=<çıktı.exe> installer.nsi
; Yönetici izni istemez: kullanıcının kendi klasörüne (%LOCALAPPDATA%\Programs) kurar.

Unicode true
ManifestDPIAware true
SetCompressor /SOLID lzma
RequestExecutionLevel user

!include "MUI2.nsh"
!include "FileFunc.nsh"
!include "LogicLib.nsh"
!include "Sections.nsh"

!ifndef VERSION
  !define VERSION "2.3.0"
!endif
!define APPNAME "Halis Adisyon"
!define APPEXE "HalisAdisyon.exe"
!define WINCLASS "webview"
!define MUTEX "Local\HalisAdisyon-tek-kopya"
!define UNINSTKEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\HalisAdisyon"
!define RUNKEY "Software\Microsoft\Windows\CurrentVersion\Run"

Name "${APPNAME}"
OutFile "${OUTFILE}"
InstallDir "$LOCALAPPDATA\Programs\Halis Adisyon"
InstallDirRegKey HKCU "Software\HalisAdisyon" "InstallDir"
BrandingText "${APPNAME} ${VERSION}"

!define MUI_ICON "${STAGE}/app.ico"
!define MUI_UNICON "${STAGE}/app.ico"
!define MUI_ABORTWARNING
!define MUI_COMPONENTSPAGE_NODESC

!define MUI_WELCOMEPAGE_TITLE "Halis Adisyon kurulumu"
!define MUI_WELCOMEPAGE_TEXT "Bu sihirbaz Halis Adisyon ${VERSION} programını bilgisayarınıza kuracak.$\r$\n$\r$\nKurulum yönetici izni ya da internet gerektirmez. Daha önce kurduysanız kayıtlarınız korunur.$\r$\n$\r$\nDevam etmek için İleri'ye basın."
!define MUI_FINISHPAGE_TITLE "Kurulum tamamlandı"
!define MUI_FINISHPAGE_TEXT "Halis Adisyon kuruldu. Masaüstündeki Halis Adisyon simgesiyle açabilirsiniz.$\r$\n$\r$\nİlk açılışta işletme bilgileri, yazıcı ve fiyatlar adım adım sorulur."
!define MUI_FINISHPAGE_RUN "$INSTDIR\${APPEXE}"
!define MUI_FINISHPAGE_RUN_TEXT "Halis Adisyon'u şimdi aç"
!define MUI_FINISHPAGE_SHOWREADME "$INSTDIR\Kullanim_Rehberi.txt"
!define MUI_FINISHPAGE_SHOWREADME_TEXT "Kısa kullanım rehberini göster"
!define MUI_FINISHPAGE_SHOWREADME_NOTCHECKED

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "Turkish"

VIProductVersion "${VERSION}.0"
VIAddVersionKey /LANG=${LANG_TURKISH} "ProductName" "${APPNAME}"
VIAddVersionKey /LANG=${LANG_TURKISH} "FileDescription" "${APPNAME} kurulumu"
VIAddVersionKey /LANG=${LANG_TURKISH} "FileVersion" "${VERSION}"
VIAddVersionKey /LANG=${LANG_TURKISH} "ProductVersion" "${VERSION}"
VIAddVersionKey /LANG=${LANG_TURKISH} "CompanyName" "Halis Erzurum Cağ Kebabı"
VIAddVersionKey /LANG=${LANG_TURKISH} "LegalCopyright" "Halis Erzurum Cağ Kebabı"

; Program açıkken dosyası değiştirilemez: kapatılmasını iste. Pencere kapanmış ama
; program hâlâ çalışıyorsa (tarayıcı modu ya da kapanış yedeği) birkaç saniye bekle.
!macro WAIT_APP_CLOSED
  StrCpy $9 0
  ${Do}
    FindWindow $0 "${WINCLASS}" "${APPNAME}"
    System::Call 'kernel32::OpenMutexW(i 0x00100000, i 0, w "${MUTEX}") p .r1'
    ${If} $1 != 0
      System::Call 'kernel32::CloseHandle(p r1)'
    ${EndIf}
    ${If} $0 == 0
    ${AndIf} $1 == 0
      ${Break}
    ${EndIf}
    ${If} $0 == 0
    ${AndIf} $9 < 24
      IntOp $9 $9 + 1
      Sleep 500
      ${Continue}
    ${EndIf}
    MessageBox MB_RETRYCANCEL|MB_ICONEXCLAMATION "Halis Adisyon şu an açık.$\r$\n$\r$\nProgramı sağ üstteki güç simgesiyle kapatıp Yeniden Dene'ye basın. Masalar kayıtlı olduğu için hiçbir şey kaybolmaz." /SD IDCANCEL IDRETRY +2
    Abort
    StrCpy $9 0
  ${Loop}
!macroend

Section "Halis Adisyon (gerekli)" SecMain
  SectionIn RO
  SetOutPath "$INSTDIR"
  File "${STAGE}/${APPEXE}"
  File "${STAGE}/Kullanim_Rehberi.txt"
  File "${STAGE}/Lisanslar.txt"
  WriteUninstaller "$INSTDIR\Kaldir.exe"

  CreateDirectory "$SMPROGRAMS\Halis Adisyon"
  CreateShortcut "$SMPROGRAMS\Halis Adisyon\Halis Adisyon.lnk" "$INSTDIR\${APPEXE}" "" "$INSTDIR\${APPEXE}" 0
  CreateShortcut "$SMPROGRAMS\Halis Adisyon\Kullanım rehberi.lnk" "$INSTDIR\Kullanim_Rehberi.txt"
  CreateShortcut "$SMPROGRAMS\Halis Adisyon\Halis Adisyon'u kaldır.lnk" "$INSTDIR\Kaldir.exe"

  WriteRegStr HKCU "Software\HalisAdisyon" "InstallDir" "$INSTDIR"
  WriteRegStr HKCU "${UNINSTKEY}" "DisplayName" "${APPNAME}"
  WriteRegStr HKCU "${UNINSTKEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${UNINSTKEY}" "Publisher" "Halis Erzurum Cağ Kebabı"
  WriteRegStr HKCU "${UNINSTKEY}" "DisplayIcon" "$INSTDIR\${APPEXE}"
  WriteRegStr HKCU "${UNINSTKEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINSTKEY}" "UninstallString" '"$INSTDIR\Kaldir.exe"'
  WriteRegDWORD HKCU "${UNINSTKEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINSTKEY}" "NoRepair" 1
  ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
  IntFmt $0 "0x%08X" $0
  WriteRegDWORD HKCU "${UNINSTKEY}" "EstimatedSize" "$0"

  ; Otomatik başlatma seçili değilse eski kaydı kaldır (bölüm seçiliyse aşağıda yeniden yazılır).
  DeleteRegValue HKCU "${RUNKEY}" "HalisAdisyon"
SectionEnd

Section "Masaüstüne simge koy" SecDesktop
  CreateShortcut "$DESKTOP\Halis Adisyon.lnk" "$INSTDIR\${APPEXE}" "" "$INSTDIR\${APPEXE}" 0
SectionEnd

Section "Windows açılınca programı başlat" SecAuto
  WriteRegStr HKCU "${RUNKEY}" "HalisAdisyon" '"$INSTDIR\${APPEXE}"'
SectionEnd

Function .onInit
  !insertmacro WAIT_APP_CLOSED
  ; Güncellemede: kullanıcı otomatik başlatmayı kapattıysa yeniden açma.
  ReadRegStr $2 HKCU "Software\HalisAdisyon" "InstallDir"
  ${If} $2 != ""
    ReadRegStr $3 HKCU "${RUNKEY}" "HalisAdisyon"
    ${If} $3 == ""
      !insertmacro UnselectSection ${SecAuto}
    ${EndIf}
  ${EndIf}
FunctionEnd

Function un.onInit
  !insertmacro WAIT_APP_CLOSED
FunctionEnd

Section "Uninstall"
  Delete "$INSTDIR\${APPEXE}"
  Delete "$INSTDIR\Kullanim_Rehberi.txt"
  Delete "$INSTDIR\Lisanslar.txt"
  Delete "$INSTDIR\Kaldir.exe"
  RMDir "$INSTDIR"
  Delete "$DESKTOP\Halis Adisyon.lnk"
  RMDir /r "$SMPROGRAMS\Halis Adisyon"
  DeleteRegValue HKCU "${RUNKEY}" "HalisAdisyon"
  DeleteRegKey HKCU "${UNINSTKEY}"
  DeleteRegKey HKCU "Software\HalisAdisyon"
  MessageBox MB_ICONINFORMATION "Halis Adisyon kaldırıldı.$\r$\n$\r$\nAdisyon kayıtları, yedekler ve Excel raporları silinmedi:$\r$\n$LOCALAPPDATA\HalisAdisyon$\r$\n$DOCUMENTS\Halis Adisyon Yedekleri$\r$\n$DOCUMENTS\Halis Adisyon Raporları" /SD IDOK
SectionEnd
