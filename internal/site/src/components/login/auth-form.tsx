import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import { getPagePath } from "@nanostores/router"
import { KeyIcon, LoaderCircle, LockIcon, LogInIcon, MailIcon } from "lucide-react"
import type { AuthMethodsList, AuthProviderInfo, OAuth2AuthConfig } from "pocketbase"
import { useCallback, useEffect, useState } from "react"
import * as v from "valibot"
import { buttonVariants } from "@/components/ui/button"
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { pb } from "@/lib/api"
import { $authenticated } from "@/lib/stores"
import { cn } from "@/lib/utils"
import { $router, Link, basePath, prependBasePath } from "../router"
import { toast } from "../ui/use-toast"
import { OtpInputForm } from "./otp-forms"

const honeypot = v.literal("")
const emailSchema = v.pipe(v.string(), v.rfcEmail(t`Invalid email address.`))
const passwordSchema = v.pipe(
        v.string(),
        v.minLength(8, t`Password must be at least 8 characters.`),
        v.maxBytes(72, t`Password must be less than 72 bytes.`)
)

const LoginSchema = v.looseObject({
        website: honeypot,
        email: emailSchema,
        password: passwordSchema,
})

const RegisterSchema = v.looseObject({
        website: honeypot,
        email: emailSchema,
        password: passwordSchema,
        passwordConfirm: passwordSchema,
})

export const showLoginFaliedToast = (description = t`Please check your credentials and try again`) => {
        toast({
                title: t`Login attempt failed`,
                description,
                variant: "destructive",
        })
}

const getAuthProviderIcon = (provider: AuthProviderInfo) => {
        let { name } = provider
        if (name.startsWith("oidc")) {
                name = "oidc"
        }
        return prependBasePath(`/_/images/oauth2/${name}.svg`)
}

export function UserAuthForm({
        className,
        isFirstRun,
        authMethods,
        ...props
}: {
        className?: string
        isFirstRun: boolean
        authMethods: AuthMethodsList
}) {
        const [isLoading, setIsLoading] = useState<boolean>(false)
        const [isOauthLoading, setIsOauthLoading] = useState<boolean>(false)
        const [errors, setErrors] = useState<Record<string, string | undefined>>({})
        const [mfaId, setMfaId] = useState<string | undefined>()
        const [otpId, setOtpId] = useState<string | undefined>()

        const handleSubmit = useCallback(
                async (e: React.FormEvent<HTMLFormElement>) => {
                        e.preventDefault()
                        // store email for later use if mfa is enabled
                        let email = ""
                        try {
                                const formData = new FormData(e.target as HTMLFormElement)
                                const data = Object.fromEntries(formData) as Record<string, any>
                                const Schema = isFirstRun ? RegisterSchema : LoginSchema
                                const result = v.safeParse(Schema, data)
                                if (!result.success) {
                                        const errors: Record<string, string> = {}
                                        for (const issue of result.issues) {
                                                const key = issue.path?.[0]?.key as string | undefined
                                                if (key && key !== "website") {
                                                        errors[key] = issue.message
                                                }
                                        }
                                        if (Object.keys(errors).length === 0) {
                                                errors["email"] = "Please check your details and try again."
                                        }
                                        setErrors(errors)
                                        return
                                }
                                setIsLoading(true)
                                const { password, passwordConfirm } = result.output
                                email = result.output.email
                                if (isFirstRun) {
                                        // check that passwords match
                                        if (password !== passwordConfirm) {
                                                const msg = "Passwords do not match"
                                                setErrors({ passwordConfirm: msg })
                                                return
                                        }
                                        await pb.send("/api/beszel/create-user", {
                                                method: "POST",
                                                body: JSON.stringify({ email, password }),
                                        })
                                        await pb.collection("users").authWithPassword(email, password)
                                } else {
                                        await pb.collection("users").authWithPassword(email, password)
                                }
                                $authenticated.set(true)
                        } catch (err: any) {
                                const mfaId = err?.response?.mfaId
                                if (!mfaId) {
                                        showLoginFaliedToast()
                                        throw err
                                }
                                setMfaId(mfaId)
                                try {
                                        const { otpId } = await pb.collection("users").requestOTP(email)
                                        setOtpId(otpId)
                                } catch (err) {
                                        console.log({ err })
                                        showLoginFaliedToast()
                                }
                        } finally {
                                setIsLoading(false)
                        }
                },
                [isFirstRun]
        )

        const authProviders = authMethods.oauth2.providers ?? []
        const oauthEnabled = authMethods.oauth2.enabled && authProviders.length > 0
        const passwordEnabled = authMethods.password.enabled
        const otpEnabled = authMethods.otp.enabled
        const mfaEnabled = authMethods.mfa.enabled

        function loginWithOauth(provider: AuthProviderInfo, forcePopup = false) {
                setIsOauthLoading(true)

                if (globalThis.BESZEL.OAUTH_DISABLE_POPUP) {
                        redirectToOauthProvider(provider)
                        return
                }

                const oAuthOpts: OAuth2AuthConfig = {
                        provider: provider.name,
                }
                // https://github.com/pocketbase/pocketbase/discussions/2429#discussioncomment-5943061
                if (forcePopup || navigator.userAgent.match(/iPhone|iPad|iPod/i)) {
                        const authWindow = window.open()
                        if (!authWindow) {
                                setIsOauthLoading(false)
                                showLoginFaliedToast(t`Please enable pop-ups for this site`)
                                return
                        }
                        oAuthOpts.urlCallback = (url) => {
                                authWindow.location.href = url
                        }
                }
                pb.collection("users")
                        .authWithOAuth2(oAuthOpts)
                        .then(() => {
                                $authenticated.set(pb.authStore.isValid)
                        })
                        .catch(showLoginFaliedToast)
                        .finally(() => {
                                setIsOauthLoading(false)
                        })
        }

        /**
         * Redirects the user to the OAuth provider's authentication page in the same window.
         * Requires the app's base URL to be registered as a redirect URI with the OAuth provider.
         */
        function redirectToOauthProvider(provider: AuthProviderInfo) {
                const url = new URL(provider.authURL)
                // url.searchParams.set("redirect_uri", `${window.location.origin}${basePath}`)
                sessionStorage.setItem("provider", JSON.stringify(provider))
                window.location.href = url.toString()
        }

        useEffect(() => {
                // handle redirect-based OAuth callback if we have a code
                const params = new URLSearchParams(window.location.search)
                const code = params.get("code")
                if (code) {
                        const state = params.get("state")
                        const provider: AuthProviderInfo = JSON.parse(sessionStorage.getItem("provider") ?? "{}")
                        if (!state || provider.state !== state) {
                                showLoginFaliedToast()
                        } else {
                                setIsOauthLoading(true)
                                window.history.replaceState({}, "", window.location.pathname)
                                pb.collection("users")
                                        .authWithOAuth2Code(provider.name, code, provider.codeVerifier, `${window.location.origin}${basePath}`)
                                        .then(() => $authenticated.set(pb.authStore.isValid))
                                        .catch((e: unknown) => showLoginFaliedToast((e as Error).message))
                                        .finally(() => setIsOauthLoading(false))
                        }
                }

                // auto login if password disabled and only one auth provider
                if (!code && !passwordEnabled && authProviders.length === 1 && !sessionStorage.getItem("lo")) {
                        // Add a small timeout to ensure browser is ready to handle popups
                        setTimeout(() => loginWithOauth(authProviders[0], false), 300)
                        return
                }

                // refresh auth if not in above states (required for trusted auth header)
                pb.collection("users")
                        .authRefresh()
                        .then((res) => {
                                pb.authStore.save(res.token, res.record)
                                $authenticated.set(!!pb.authStore.isValid)
