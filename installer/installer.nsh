; The native messaging host the browser extension talks through is
; registered per user under HKCU, and the manifest lives in %APPDATA%\Gale.
; Uninstalling the app removes every browser key, so the browser never calls
; a host that is gone.
!macro customUnInstall
  DeleteRegKey HKCU "Software\Google\Chrome\NativeMessagingHosts\app.gale.host"
  DeleteRegKey HKCU "Software\Microsoft\Edge\NativeMessagingHosts\app.gale.host"
  DeleteRegKey HKCU "Software\BraveSoftware\Brave-Browser\NativeMessagingHosts\app.gale.host"
  DeleteRegKey HKCU "Software\Vivaldi\NativeMessagingHosts\app.gale.host"
  DeleteRegKey HKCU "Software\Opera Software\NativeMessagingHosts\app.gale.host"
  DeleteRegKey HKCU "Software\Chromium\NativeMessagingHosts\app.gale.host"
  RMDir /r "$APPDATA\Gale 下载器\native"
  RMDir /r "$APPDATA\Gale\native"
!macroend
