package com.familyvpn.app.data

import android.content.Context
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map

private val Context.dataStore by preferencesDataStore("family_vpn")

class ConfigStore(private val context: Context) {
    private val keyConfig = stringPreferencesKey("awg_config")
    private val keyEmail = stringPreferencesKey("account_email")

    val configFlow: Flow<String> = context.dataStore.data.map { it[keyConfig].orEmpty() }
    val emailFlow: Flow<String> = context.dataStore.data.map { it[keyEmail].orEmpty() }

    suspend fun saveConfig(raw: String) {
        context.dataStore.edit { it[keyConfig] = raw.trim() + "\n" }
    }

    suspend fun saveEmail(email: String) {
        context.dataStore.edit { it[keyEmail] = email.trim() }
    }

    suspend fun getConfig(): String = configFlow.first()

    suspend fun clearConfig() {
        context.dataStore.edit {
            it.remove(keyConfig)
        }
    }

    fun looksLikeConfig(raw: String): Boolean {
        val t = raw.trim()
        return t.contains("[Interface]", ignoreCase = true) &&
            t.contains("[Peer]", ignoreCase = true) &&
            t.contains("PrivateKey", ignoreCase = true)
    }
}
