import React, { useState, useEffect, useRef } from "react"
import {
  StyleSheet,
  Text,
  View,
  TextInput,
  TouchableOpacity,
  SafeAreaView,
  StatusBar,
  ActivityIndicator,
  Alert,
  KeyboardAvoidingView,
  Platform,
  BackHandler,
} from "react-native"
import AsyncStorage from "@react-native-async-storage/async-storage"
import { WebView } from "react-native-webview"
import * as Device from "expo-device"
import * as Notifications from "expo-notifications"

// Configure notification behavior
Notifications.setNotificationHandler({
  handleNotification: async () => ({
    shouldShowAlert: true,
    shouldPlaySound: true,
    shouldSetBadge: false,
  }),
})

const DEFAULT_URL = "https://hub.unitaryx.org"
const HUB_URL_KEY = "@hub_url"
const DEVICE_ID_KEY = "@device_id"

export default function App() {
  const [hubUrl, setHubUrl] = useState("")
  const [inputUrl, setInputUrl] = useState(DEFAULT_URL)
  const [loading, setLoading] = useState(true)
  const [connecting, setConnecting] = useState(false)
  const [deviceId, setDeviceId] = useState("")
  const [expoPushToken, setExpoPushToken] = useState("")

  const webViewRef = useRef<WebView>(null)
  const lastRegisteredToken = useRef("")

  // Load saved configurations on startup
  useEffect(() => {
    async function loadConfig() {
      try {
        const savedUrl = await AsyncStorage.getItem(HUB_URL_KEY)
        const savedDeviceId = await AsyncStorage.getItem(DEVICE_ID_KEY)

        if (savedUrl) {
          setHubUrl(savedUrl)
          setInputUrl(savedUrl)
        }

        if (savedDeviceId) {
          setDeviceId(savedDeviceId)
        } else {
          // Generate a simple unique ID for this device installation
          const newId = Math.random().toString(36).substring(2, 15) + Math.random().toString(36).substring(2, 15)
          await AsyncStorage.setItem(DEVICE_ID_KEY, newId)
          setDeviceId(newId)
        }
      } catch (err) {
        console.error("Failed to load AsyncStorage configurations", err)
      } finally {
        setLoading(false)
      }
    }
    loadConfig()
  }, [])

  // Register for push notifications
  useEffect(() => {
    async function registerForPushNotifications() {
      if (!Device.isDevice) {
        console.log("Must use physical device for Push Notifications")
        return
      }

      try {
        const { status: existingStatus } = await Notifications.getPermissionsAsync()
        let finalStatus = existingStatus

        if (existingStatus !== "granted") {
          const { status } = await Notifications.requestPermissionsAsync()
          finalStatus = status
        }

        if (finalStatus !== "granted") {
          console.log("Failed to get push token for push notifications!")
          return
        }

        const tokenData = await Notifications.getExpoPushTokenAsync()
        setExpoPushToken(tokenData.data)
      } catch (error) {
        console.error("Error fetching Expo push token", error)
      }
    }

    registerForPushNotifications()
  }, [])

  // Handle Android back button to navigate back in WebView history
  useEffect(() => {
    const onBackPress = () => {
      if (webViewRef.current && hubUrl) {
        webViewRef.current.goBack()
        return true
      }
      return false
    }

    BackHandler.addEventListener("hardwareBackPress", onBackPress)
    return () => BackHandler.removeEventListener("hardwareBackPress", onBackPress)
  }, [hubUrl])

  // Save the connection URL
  const handleConnect = async () => {
    let formattedUrl = inputUrl.trim()
    if (!formattedUrl) {
      Alert.alert("Invalid URL", "Please enter a valid URL.")
      return
    }

    if (!/^https?:\/\//i.test(formattedUrl)) {
      formattedUrl = "https://" + formattedUrl
    }

    // Strip trailing slash
    formattedUrl = formattedUrl.replace(/\/$/, "")

    setConnecting(true)
    try {
      await AsyncStorage.setItem(HUB_URL_KEY, formattedUrl)
      setHubUrl(formattedUrl)
    } catch (err) {
      Alert.alert("Error", "Failed to save URL.")
    } finally {
      setConnecting(false)
    }
  }

  // Register device push token to backend via PocketBase API
  const registerDeviceTokenWithBackend = async (userId: string, token: string) => {
    if (!expoPushToken || !deviceId || !hubUrl) return
    const registrationKey = `${userId}:${expoPushToken}`

    // Avoid duplicate requests for the same session token
    if (lastRegisteredToken.current === registrationKey) return
    lastRegisteredToken.current = registrationKey

    try {
      // Check if this device is already registered
      const filter = encodeURIComponent(`user='${userId}' && device_id='${deviceId}'`)
      const response = await fetch(
        `${hubUrl}/api/collections/mobile_devices/records?filter=(${filter})`,
        {
          headers: {
            Authorization: `Bearer ${token}`,
          },
        }
      )

      if (!response.ok) {
        throw new Error(`Failed to query device registrations: ${response.statusText}`)
      }

      const listData = await response.json()

      if (listData.items && listData.items.length > 0) {
        // Record exists, check if token needs updating
        const record = listData.items[0]
        if (record.token !== expoPushToken) {
          const updateResponse = await fetch(
            `${hubUrl}/api/collections/mobile_devices/records/${record.id}`,
            {
              method: "PATCH",
              headers: {
                "Content-Type": "application/json",
                Authorization: `Bearer ${token}`,
              },
              body: JSON.stringify({ token: expoPushToken }),
            }
          )
          if (!updateResponse.ok) {
            console.error("Failed to update push token record")
          } else {
            console.log("Expo push token updated successfully")
          }
        }
      } else {
        // Create new registration record
        const createResponse = await fetch(
          `${hubUrl}/api/collections/mobile_devices/records`,
          {
            method: "POST",
            headers: {
              "Content-Type": "application/json",
              Authorization: `Bearer ${token}`,
            },
            body: JSON.stringify({
              user: userId,
              token: expoPushToken,
              device_id: deviceId,
            }),
          }
        )
        if (!createResponse.ok) {
          console.error("Failed to create push token record")
        } else {
          console.log("Expo push token registered successfully")
        }
      }
    } catch (err) {
      console.error("Error registering device token with backend", err)
      // Reset tracker so it can retry
      lastRegisteredToken.current = ""
    }
  }

  // Handle messages sent from WebView
  const handleWebViewMessage = (event: any) => {
    try {
      const message = JSON.parse(event.nativeEvent.data)
      if (message.type === "AUTH_SUCCESS") {
        registerDeviceTokenWithBackend(message.userId, message.token)
      } else if (message.type === "RESET_HUB_URL") {
        handleDisconnect()
      }
    } catch (e) {
      // Silent error for non-JSON payloads
    }
  }

  // Disconnect from current Hub and reset URL
  const handleDisconnect = async () => {
    Alert.alert(
      "Disconnect",
      "Are you sure you want to disconnect and configure a new Hub URL?",
      [
        { text: "Cancel", style: "cancel" },
        {
          text: "Disconnect",
          style: "destructive",
          onPress: async () => {
            try {
              await AsyncStorage.removeItem(HUB_URL_KEY)
              setHubUrl("")
              lastRegisteredToken.current = ""
            } catch (err) {
              console.error("Failed to disconnect", err)
            }
          },
        },
      ]
    )
  }

  // Script injected into WebView to read PocketBase auth state and expose reset hook
  const injectedJsScript = `
    (function() {
      // Expose a native log out / URL configuration trigger
      window.resetMobileHubUrl = function() {
        window.ReactNativeWebView.postMessage(JSON.stringify({ type: 'RESET_HUB_URL' }));
      };

      function checkPocketBaseAuth() {
        try {
          const authData = localStorage.getItem('pocketbase_auth');
          if (authData) {
            const parsed = JSON.parse(authData);
            if (parsed && parsed.token && parsed.model && parsed.model.id) {
              window.ReactNativeWebView.postMessage(JSON.stringify({
                type: 'AUTH_SUCCESS',
                userId: parsed.model.id,
                token: parsed.token
              }));
            }
          }
        } catch (e) {
          // Ignore storage errors
        }
      }

      // Check immediately
      checkPocketBaseAuth();
      // Check periodically in case user logs in later
      setInterval(checkPocketBaseAuth, 3000);
    })();
    true;
  `

  if (loading) {
    return (
      <View style={styles.loadingContainer}>
        <ActivityIndicator size="large" color="#a855f7" />
      </View>
    )
  }

  // If no Hub URL is saved, display the configuration setup screen
  if (!hubUrl) {
    return (
      <SafeAreaView style={styles.safeArea}>
        <StatusBar barStyle="light-content" backgroundColor="#121316" />
        <KeyboardAvoidingView
          behavior={Platform.OS === "ios" ? "padding" : "height"}
          style={styles.keyboardContainer}
        >
          <View style={styles.configContainer}>
            <View style={styles.logoContainer}>
              <Text style={styles.logoTextTitle}>Beszel<Text style={styles.logoAccent}>X</Text>Harix</Text>
              <Text style={styles.logoSubtitle}>Premium Resource Monitoring</Text>
            </View>

            <View style={styles.card}>
              <Text style={styles.label}>Hub Server URL</Text>
              <TextInput
                style={styles.input}
                value={inputUrl}
                onChangeText={setInputUrl}
                placeholder="https://hub.unitaryx.org"
                placeholderTextColor="#64748b"
                autoCapitalize="none"
                autoCorrect={false}
                keyboardType="url"
              />
              <Text style={styles.helperText}>
                Enter the domain name or IP address of your Beszel dashboard hub.
              </Text>

              <TouchableOpacity
                style={styles.connectButton}
                onPress={handleConnect}
                disabled={connecting}
              >
                {connecting ? (
                  <ActivityIndicator color="#ffffff" />
                ) : (
                  <Text style={styles.connectButtonText}>Connect to Hub</Text>
                )}
              </TouchableOpacity>
            </View>
          </View>
        </KeyboardAvoidingView>
      </SafeAreaView>
    )
  }

  // Render WebView displaying Beszel dashboard
  return (
    <SafeAreaView style={styles.webviewSafeArea}>
      <StatusBar barStyle="light-content" backgroundColor="#0f172a" />
      <View style={styles.webviewHeader}>
        <Text style={styles.headerTitle}>BeszelXHarix</Text>
        <TouchableOpacity style={styles.disconnectHeaderButton} onPress={handleDisconnect}>
          <Text style={styles.disconnectHeaderText}>Change Hub</Text>
        </TouchableOpacity>
      </View>
      <WebView
        ref={webViewRef}
        source={{ uri: hubUrl }}
        injectedJavaScript={injectedJsScript}
        onMessage={handleWebViewMessage}
        style={styles.webview}
        originWhitelist={['*']}
        mixedContentMode="always"
        userAgent="Mozilla/5.0 (Linux; Android 10; Mobile) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Mobile Safari/537.36"
        domStorageEnabled={true}
        javaScriptEnabled={true}
        allowsBackForwardNavigationGestures={true}
        startInLoadingState={true}
        onError={(syntheticEvent) => {
          const { nativeEvent } = syntheticEvent
          console.warn("WebView error: ", nativeEvent)
        }}
        renderLoading={() => (
          <View style={styles.webviewLoading}>
            <ActivityIndicator size="large" color="#a855f7" />
          </View>
        )}
        renderError={(errorName) => (
          <View style={styles.errorContainer}>
            <Text style={styles.errorTitle}>Unable to connect to Hub</Text>
            <Text style={styles.errorSubtext}>
              Could not reach <Text style={styles.errorUrl}>{hubUrl}</Text>.
            </Text>
            <Text style={styles.errorHelp}>
              • Check your internet connection or server status{"\n"}
              • If using local hub, use http://10.0.2.2:8090
            </Text>
            <TouchableOpacity
              style={styles.retryButton}
              onPress={() => webViewRef.current?.reload()}
            >
              <Text style={styles.retryButtonText}>Retry Connection</Text>
            </TouchableOpacity>
            <TouchableOpacity
              style={styles.changeUrlButton}
              onPress={() => setHubUrl("")}
            >
              <Text style={styles.changeUrlButtonText}>Change Hub URL</Text>
            </TouchableOpacity>
          </View>
        )}
      />
    </SafeAreaView>
  )
}

const styles = StyleSheet.create({
  safeArea: {
    flex: 1,
    backgroundColor: "#121316",
  },
  webviewSafeArea: {
    flex: 1,
    backgroundColor: "#0f172a",
  },
  loadingContainer: {
    flex: 1,
    backgroundColor: "#121316",
    justifyContent: "center",
    alignItems: "center",
  },
  keyboardContainer: {
    flex: 1,
  },
  configContainer: {
    flex: 1,
    justifyContent: "center",
    paddingHorizontal: 24,
  },
  logoContainer: {
    alignItems: "center",
    marginBottom: 40,
  },
  logoTextTitle: {
    fontSize: 36,
    fontWeight: "bold",
    color: "#ffffff",
    letterSpacing: 1.5,
  },
  logoAccent: {
    color: "#a855f7",
    fontWeight: "900",
  },
  logoSubtitle: {
    fontSize: 14,
    color: "#94a3b8",
    marginTop: 8,
    textTransform: "uppercase",
    letterSpacing: 2,
  },
  card: {
    backgroundColor: "#1e1e24",
    borderRadius: 16,
    padding: 24,
    shadowColor: "#000",
    shadowOffset: { width: 0, height: 4 },
    shadowOpacity: 0.3,
    shadowRadius: 8,
    elevation: 5,
    borderWidth: 1,
    borderColor: "#2e2e38",
  },
  label: {
    fontSize: 14,
    fontWeight: "600",
    color: "#e2e8f0",
    marginBottom: 8,
  },
  input: {
    backgroundColor: "#121316",
    color: "#ffffff",
    borderRadius: 8,
    paddingHorizontal: 16,
    paddingVertical: 12,
    fontSize: 16,
    borderWidth: 1,
    borderColor: "#3f3f46",
    marginBottom: 8,
  },
  helperText: {
    fontSize: 12,
    color: "#94a3b8",
    marginBottom: 24,
    lineHeight: 16,
  },
  connectButton: {
    backgroundColor: "#a855f7",
    borderRadius: 8,
    paddingVertical: 14,
    alignItems: "center",
    shadowColor: "#a855f7",
    shadowOffset: { width: 0, height: 2 },
    shadowOpacity: 0.4,
    shadowRadius: 4,
    elevation: 3,
  },
  connectButtonText: {
    color: "#ffffff",
    fontSize: 16,
    fontWeight: "bold",
  },
  webview: {
    flex: 1,
    backgroundColor: "#0f172a",
  },
  webviewHeader: {
    height: 50,
    backgroundColor: "#1e293b",
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingHorizontal: 16,
    borderBottomWidth: 1,
    borderBottomColor: "#334155",
  },
  headerTitle: {
    color: "#ffffff",
    fontSize: 16,
    fontWeight: "bold",
  },
  disconnectHeaderButton: {
    backgroundColor: "#334155",
    paddingVertical: 6,
    paddingHorizontal: 12,
    borderRadius: 6,
  },
  disconnectHeaderText: {
    color: "#cbd5e1",
    fontSize: 12,
    fontWeight: "600",
  },
  webviewLoading: {
    ...StyleSheet.absoluteFillObject,
    backgroundColor: "#0f172a",
    justifyContent: "center",
    alignItems: "center",
  },
  errorContainer: {
    ...StyleSheet.absoluteFillObject,
    backgroundColor: "#0f172a",
    justifyContent: "center",
    alignItems: "center",
    paddingHorizontal: 24,
  },
  errorTitle: {
    fontSize: 20,
    fontWeight: "bold",
    color: "#f87171",
    marginBottom: 12,
  },
  errorSubtext: {
    fontSize: 14,
    color: "#94a3b8",
    textAlign: "center",
    marginBottom: 16,
  },
  errorUrl: {
    color: "#ffffff",
    fontWeight: "bold",
  },
  errorHelp: {
    fontSize: 12,
    color: "#64748b",
    lineHeight: 20,
    marginBottom: 28,
    textAlign: "center",
  },
  retryButton: {
    backgroundColor: "#a855f7",
    paddingVertical: 12,
    paddingHorizontal: 24,
    borderRadius: 8,
    width: "100%",
    alignItems: "center",
    marginBottom: 12,
  },
  retryButtonText: {
    color: "#ffffff",
    fontSize: 15,
    fontWeight: "bold",
  },
  changeUrlButton: {
    backgroundColor: "#1e293b",
    paddingVertical: 12,
    paddingHorizontal: 24,
    borderRadius: 8,
    width: "100%",
    alignItems: "center",
    borderWidth: 1,
    borderColor: "#334155",
  },
  changeUrlButtonText: {
    color: "#cbd5e1",
    fontSize: 15,
    fontWeight: "600",
  },
})
