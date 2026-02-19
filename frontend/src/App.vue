<script setup>
import ConnectionDialog from './components/dialogs/ConnectionDialog.vue'
import NewKeyDialog from './components/dialogs/NewKeyDialog.vue'
import PreferencesDialog from './components/dialogs/PreferencesDialog.vue'
import RenameKeyDialog from './components/dialogs/RenameKeyDialog.vue'
import SetTtlDialog from './components/dialogs/SetTtlDialog.vue'
import AddFieldsDialog from './components/dialogs/AddFieldsDialog.vue'
import AppContent from './AppContent.vue'
import GroupDialog from './components/dialogs/GroupDialog.vue'
import DeleteKeyDialog from './components/dialogs/DeleteKeyDialog.vue'
import { h, onMounted, onUnmounted, ref, watch } from 'vue'
import usePreferencesStore from './stores/preferences.js'
import useConnectionStore from './stores/connections.js'
import { useI18n } from 'vue-i18n'
import { darkTheme, NButton, NSpace } from 'naive-ui'
import KeyFilterDialog from './components/dialogs/KeyFilterDialog.vue'
import { Environment, WindowSetDarkTheme, WindowSetLightTheme, ReconnectWebSocket } from 'wailsjs/runtime/runtime.js'
import { darkThemeOverrides, themeOverrides } from '@/utils/theme.js'
import AboutDialog from '@/components/dialogs/AboutDialog.vue'
import FlushDbDialog from '@/components/dialogs/FlushDbDialog.vue'
import ExportKeyDialog from '@/components/dialogs/ExportKeyDialog.vue'
import ImportKeyDialog from '@/components/dialogs/ImportKeyDialog.vue'
import { Info } from 'wailsjs/go/services/systemService.js'
import DecoderDialog from '@/components/dialogs/DecoderDialog.vue'
import { loadModule, trackEvent } from '@/utils/analytics.js'
import LoginPage from '@/components/LoginPage.vue'

const prefStore = usePreferencesStore()
const connectionStore = useConnectionStore()
const i18n = useI18n()
const initializing = ref(true)

// Auth state
const authChecking = ref(true)
const authenticated = ref(false)
const authEnabled = ref(false)

const checkAuth = async () => {
    try {
        const resp = await fetch('/api/auth/status', { credentials: 'same-origin' })
        const result = await resp.json()
        if (result.success) {
            authEnabled.value = result.data.enabled
            authenticated.value = result.data.authenticated
        }
    } catch {
        authenticated.value = false
    } finally {
        authChecking.value = false
    }
}

const onLogin = async () => {
    authenticated.value = true
    // Reconnect WebSocket with auth cookie now available (non-blocking)
    ReconnectWebSocket()
    await initApp()
}

const initApp = async () => {
    try {
        initializing.value = true
        await prefStore.loadPreferences()
        i18n.locale.value = prefStore.currentLanguage
        await prefStore.loadFontList()
        await prefStore.loadBuildInDecoder()
        await connectionStore.initConnections()
        if (prefStore.autoCheckUpdate) {
            prefStore.checkForUpdate()
        }
        const env = await Environment()
        loadModule(env.buildType !== 'dev' && prefStore.general.allowTrack !== false).then(() => {
            Info().then(({ data }) => {
                trackEvent('startup', data, true)
            })
        })

        // show greetings and user behavior tracking statements
        if (!!!prefStore.behavior.welcomed) {
            const n = $notification.show({
                title: () => i18n.t('dialogue.welcome.title'),
                content: () => i18n.t('dialogue.welcome.content'),
                keepAliveOnHover: true,
                closable: false,
                meta: ' ',
                action: () =>
                    h(
                        NSpace,
                        {},
                        {
                            default: () => [
                                h(
                                    NButton,
                                    {
                                        secondary: true,
                                        type: 'tertiary',
                                        onClick: () => {
                                            prefStore.setAsWelcomed(false)
                                            n.destroy()
                                        },
                                    },
                                    {
                                        default: () => i18n.t('dialogue.welcome.reject'),
                                    },
                                ),
                                h(
                                    NButton,
                                    {
                                        secondary: true,
                                        type: 'primary',
                                        onClick: () => {
                                            prefStore.setAsWelcomed(true)
                                            n.destroy()
                                        },
                                    },
                                    {
                                        default: () => i18n.t('dialogue.welcome.accept'),
                                    },
                                ),
                            ],
                        },
                    ),
            })
        }
    } finally {
        initializing.value = false
    }
}

const onUnauthorized = () => {
    if (authEnabled.value) {
        authenticated.value = false
    }
}

onMounted(async () => {
    window.addEventListener('rdm:unauthorized', onUnauthorized)
    await checkAuth()
    if (authenticated.value) {
        await initApp()
    }
})

onUnmounted(() => {
    window.removeEventListener('rdm:unauthorized', onUnauthorized)
})

// watch theme and dynamically switch
watch(
    () => prefStore.isDark,
    (isDark) => (isDark ? WindowSetDarkTheme() : WindowSetLightTheme()),
)

// watch language and dynamically switch
watch(
    () => prefStore.general.language,
    (lang) => (i18n.locale.value = prefStore.currentLanguage),
)
</script>

<template>
    <n-config-provider
        :inline-theme-disabled="true"
        :locale="prefStore.themeLocale"
        :theme="prefStore.isDark ? darkTheme : undefined"
        :theme-overrides="prefStore.isDark ? darkThemeOverrides : themeOverrides"
        class="fill-height">
        <!-- Auth gate -->
        <template v-if="authChecking">
            <div style="width: 100vw; height: 100vh"></div>
        </template>
        <template v-else-if="authEnabled && !authenticated">
            <login-page @login="onLogin" />
        </template>
        <template v-else>
            <n-dialog-provider>
                <app-content :loading="initializing" />

                <!-- top modal dialogs -->
                <connection-dialog />
                <group-dialog />
                <new-key-dialog />
                <key-filter-dialog />
                <add-fields-dialog />
                <rename-key-dialog />
                <delete-key-dialog />
                <export-key-dialog />
                <import-key-dialog />
                <flush-db-dialog />
                <set-ttl-dialog />
                <preferences-dialog />
                <decoder-dialog />
                <about-dialog />
            </n-dialog-provider>
        </template>
    </n-config-provider>
</template>

<style lang="scss"></style>
