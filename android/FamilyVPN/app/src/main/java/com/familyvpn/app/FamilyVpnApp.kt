package com.familyvpn.app

import android.app.Application
import com.familyvpn.app.data.ConfigStore
import com.familyvpn.app.vpn.TunnelController

class FamilyVpnApp : Application() {
    lateinit var configStore: ConfigStore
        private set
    lateinit var tunnelController: TunnelController
        private set

    override fun onCreate() {
        super.onCreate()
        instance = this
        configStore = ConfigStore(this)
        tunnelController = TunnelController(this, configStore)
    }

    companion object {
        lateinit var instance: FamilyVpnApp
            private set
    }
}
