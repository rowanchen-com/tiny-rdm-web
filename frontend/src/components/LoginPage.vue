<script setup>
import { ref, computed } from 'vue'
import { useThemeVars } from 'naive-ui'
import iconUrl from '@/assets/images/icon.png'

const themeVars = useThemeVars()
const emit = defineEmits(['login'])

// i18n for login page - detect browser language
const langTexts = {
    zh: { title: '登录', username: '用户名', password: '密码', usernamePh: '请输入用户名', passwordPh: '请输入密码', submit: '登 录', tooMany: '尝试次数过多，请稍后再试', failed: '用户名或密码错误', network: '网络错误' },
    tw: { title: '登入', username: '使用者名稱', password: '密碼', usernamePh: '請輸入使用者名稱', passwordPh: '請輸入密碼', submit: '登 入', tooMany: '嘗試次數過多，請稍後再試', failed: '使用者名稱或密碼錯誤', network: '網路錯誤' },
    ja: { title: 'ログイン', username: 'ユーザー名', password: 'パスワード', usernamePh: 'ユーザー名を入力', passwordPh: 'パスワードを入力', submit: 'ログイン', tooMany: '試行回数が多すぎます', failed: 'ユーザー名またはパスワードが正しくありません', network: 'ネットワークエラー' },
    ko: { title: '로그인', username: '사용자 이름', password: '비밀번호', usernamePh: '사용자 이름 입력', passwordPh: '비밀번호 입력', submit: '로그인', tooMany: '시도 횟수 초과, 잠시 후 다시 시도하세요', failed: '사용자 이름 또는 비밀번호가 올바르지 않습니다', network: '네트워크 오류' },
    es: { title: 'Iniciar sesión', username: 'Usuario', password: 'Contraseña', usernamePh: 'Ingrese usuario', passwordPh: 'Ingrese contraseña', submit: 'Entrar', tooMany: 'Demasiados intentos, intente más tarde', failed: 'Credenciales inválidas', network: 'Error de red' },
    fr: { title: 'Connexion', username: "Nom d'utilisateur", password: 'Mot de passe', usernamePh: "Entrez le nom d'utilisateur", passwordPh: 'Entrez le mot de passe', submit: 'Se connecter', tooMany: 'Trop de tentatives, réessayez plus tard', failed: 'Identifiants invalides', network: 'Erreur réseau' },
    ru: { title: 'Вход', username: 'Имя пользователя', password: 'Пароль', usernamePh: 'Введите имя пользователя', passwordPh: 'Введите пароль', submit: 'Войти', tooMany: 'Слишком много попыток, попробуйте позже', failed: 'Неверные учётные данные', network: 'Ошибка сети' },
    pt: { title: 'Entrar', username: 'Usuário', password: 'Senha', usernamePh: 'Digite o usuário', passwordPh: 'Digite a senha', submit: 'Entrar', tooMany: 'Muitas tentativas, tente novamente mais tarde', failed: 'Credenciais inválidas', network: 'Erro de rede' },
    tr: { title: 'Giriş', username: 'Kullanıcı adı', password: 'Şifre', usernamePh: 'Kullanıcı adını girin', passwordPh: 'Şifreyi girin', submit: 'Giriş Yap', tooMany: 'Çok fazla deneme, lütfen daha sonra tekrar deneyin', failed: 'Geçersiz kimlik bilgileri', network: 'Ağ hatası' },
    en: { title: 'Sign In', username: 'Username', password: 'Password', usernamePh: 'Enter username', passwordPh: 'Enter password', submit: 'Sign In', tooMany: 'Too many attempts, please try later', failed: 'Invalid credentials', network: 'Network error' },
}

const detectLang = () => {
    const sysLang = (navigator.language || '').toLowerCase()
    if (sysLang.startsWith('zh-tw') || sysLang.startsWith('zh-hant')) return 'tw'
    const prefix = sysLang.split('-')[0]
    return langTexts[prefix] ? prefix : 'en'
}

const t = computed(() => langTexts[detectLang()])

const username = ref('')
const password = ref('')
const loading = ref(false)
const errorMsg = ref('')
const appVersion = ref('')

// Fetch version on mount
;(async () => {
    try {
        const resp = await fetch('/api/version')
        const result = await resp.json()
        if (result.success && result.data?.version) {
            appVersion.value = result.data.version
        }
    } catch {}
})()

const canSubmit = computed(() => username.value.length > 0 && password.value.length > 0)

const handleLogin = async () => {
    if (!canSubmit.value || loading.value) return
    loading.value = true
    errorMsg.value = ''

    try {
        const resp = await fetch('/api/auth/login', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'same-origin',
            body: JSON.stringify({
                username: username.value,
                password: password.value,
            }),
        })

        const data = await resp.json()

        if (resp.status === 429) {
            errorMsg.value = t.value.tooMany
            return
        }

        if (!data.success) {
            errorMsg.value = t.value.failed
            return
        }

        emit('login')
    } catch (e) {
        errorMsg.value = t.value.network
    } finally {
        loading.value = false
    }
}
</script>

<template>
    <div class="login-wrapper">
        <div class="login-card">
            <div class="login-header">
                <n-avatar :size="48" :src="iconUrl" color="#0000" />
                <div class="login-title">Tiny RDM</div>
                <n-text depth="2" style="font-size: 13px">Redis Web Manager</n-text>
            </div>

            <n-form class="login-form" @submit.prevent="handleLogin">
                <n-form-item :label="t.username">
                    <n-input
                        v-model:value="username"
                        autofocus
                        :placeholder="t.usernamePh"
                        size="medium"
                        @keydown.enter="handleLogin" />
                </n-form-item>
                <n-form-item :label="t.password">
                    <n-input
                        v-model:value="password"
                        :placeholder="t.passwordPh"
                        show-password-on="click"
                        size="medium"
                        type="password"
                        @keydown.enter="handleLogin" />
                </n-form-item>

                <n-text v-if="errorMsg" type="error" style="font-size: 13px; margin-bottom: 12px; display: block">
                    {{ errorMsg }}
                </n-text>

                <n-button
                    :disabled="!canSubmit"
                    :loading="loading"
                    attr-type="submit"
                    block
                    type="primary"
                    size="medium"
                    @click="handleLogin">
                    {{ t.submit }}
                </n-button>
            </n-form>
        </div>

    </div>

    <div class="login-footer">
        <n-text depth="2" style="font-size: 14px">
            <span v-if="appVersion">{{ appVersion }}</span>
            <span v-if="appVersion"> · </span>
            <a
                href="https://github.com/tiny-craft/tiny-rdm"
                target="_blank"
                rel="noopener noreferrer"
                class="footer-link">
                GitHub
            </a>
        </n-text>
    </div>
</template>

<style lang="scss" scoped>
.login-wrapper {
    width: 100vw;
    height: 100vh;
    display: flex;
    align-items: center;
    justify-content: center;
    background-color: v-bind('themeVars.bodyColor');
    padding: 16px;
    box-sizing: border-box;
}

.login-card {
    width: 360px;
    max-width: 100%;
    padding: 40px 36px 36px;
    border-radius: 8px;
    border: 1px solid v-bind('themeVars.borderColor');
    background-color: v-bind('themeVars.cardColor');
    box-sizing: border-box;
}

.login-header {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 6px;
    margin-bottom: 32px;
}

.login-title {
    font-size: 22px;
    font-weight: 800;
    margin-top: 4px;
    color: v-bind('themeVars.textColor1');
}

.login-form {
    :deep(.n-form-item) {
        margin-bottom: 16px;
    }

    :deep(.n-form-item-label) {
        color: v-bind('themeVars.textColor1');
        font-weight: 500;
    }
}

.login-footer {
    position: fixed;
    bottom: 16px;
    left: 0;
    right: 0;
    text-align: center;
    color: v-bind('themeVars.textColor3');
}

.footer-link {
    color: inherit;
    text-decoration: none;
    opacity: 0.7;
    transition: opacity 0.2s;

    &:hover {
        opacity: 1;
        text-decoration: underline;
    }
}

@media (max-width: 480px) {
    .login-wrapper {
        align-items: flex-start;
        padding-top: 15vh;
    }

    .login-card {
        padding: 28px 20px 24px;
        border: none;
        border-radius: 12px;
    }

    .login-header {
        margin-bottom: 24px;
    }

    .login-footer {
        bottom: 12px;
    }
}
</style>
