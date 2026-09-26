# kotlinx.serialization
-keepattributes *Annotation*, InnerClasses
-keepclassmembers @kotlinx.serialization.Serializable class app.tunnelkey.** {
    *** Companion;
    kotlinx.serialization.KSerializer serializer(...);
}
