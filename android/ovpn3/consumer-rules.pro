# Methods invoked from native code by name.
-keep interface app.tunnelkey.ovpn3.OpenVpnClient$Callbacks { *; }
-keep class * implements app.tunnelkey.ovpn3.OpenVpnClient$Callbacks { *; }
-keepclasseswithmembernames class app.tunnelkey.ovpn3.OpenVpnClient { native <methods>; }
