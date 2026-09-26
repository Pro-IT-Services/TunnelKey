package app.tunnelkey

import android.app.Application
import app.tunnelkey.data.ProfileRepository
import app.tunnelkey.data.SecretStore
import app.tunnelkey.managed.ManagedController

class TunnelkeyApp : Application() {
    val profiles: ProfileRepository by lazy { ProfileRepository(this, SecretStore(this)) }
    val managed: ManagedController by lazy { ManagedController(this, profiles) }
}
