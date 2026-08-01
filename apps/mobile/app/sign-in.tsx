// Native Clerk sign-in — Google SSO or an email one-time code. Either path
// ends the same way: mint a durable omk_ API key from the Clerk session,
// sign the Clerk session back out (the app never keeps a live Clerk session;
// the minted key is the only credential it stores), then persist it exactly
// like a manual Settings save. By default the publishable key points at the
// hosted instance's Clerk, so sign-in works out of the box; self-hosters on an
// instance without Clerk build with an empty EXPO_PUBLIC_CLERK_PUBLISHABLE_KEY
// (falsy) and only ever see the manual fallback below. The top-level screen
// never calls a Clerk hook — those live in ClerkSignIn, which only mounts when
// a publishable key is configured, so there's no ClerkProvider to violate the
// Rules of Hooks against. Never log the Clerk token or the omk_ key.
import type { EmailCodeFactor, SignInFirstFactor } from "@clerk/types";
import { isClerkAPIResponseError, useAuth, useSignIn, useSSO } from "@clerk/clerk-expo";
import * as AuthSession from "expo-auth-session";
import { Link, useRouter } from "expo-router";
import { useEffect, useState } from "react";
import {
  ActivityIndicator,
  KeyboardAvoidingView,
  Platform,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import * as WebBrowser from "expo-web-browser";
import { PressScale } from "@/components/PressScale";
import { mintDeviceKey } from "@/lib/api";
import { clerkPublishableKey, defaultInstanceUrl } from "@/lib/clerk";
import { useSettingsContext } from "@/lib/settings-context";
import { colors, fonts, radius, spacing } from "@/lib/theme";

// Required by Clerk's useSSO flow: without this the in-app browser session
// used for OAuth may not dismiss/resolve when control returns to the app.
WebBrowser.maybeCompleteAuthSession();

type Pending = "google" | "email-request" | "email-verify" | "connecting" | null;

const GENERIC_CONNECT_ERROR = "Couldn't finish sign-in — try again.";

function isEmailCodeFactor(factor: SignInFirstFactor): factor is EmailCodeFactor {
  return factor.strategy === "email_code";
}

// Appends a short machine-readable cause to a user-facing message. Clerk
// rejections carry an `errors[0].code` (e.g. "oauth_callback_invalid",
// "identifier_already_signed_up") that names the failure precisely; without it
// every distinct cause collapses into the same opaque sentence, which is
// exactly what made this flow undiagnosable from a TestFlight build. The code
// is safe to show — it never contains the token, the email, or the key.
function withCause(message: string, err: unknown): string {
  if (isClerkAPIResponseError(err)) {
    const first = err.errors?.[0];
    if (first?.code) return `${message} (${first.code})`;
  }
  if (err instanceof Error && err.message) return `${message} (${err.message})`;
  return message;
}

export default function SignInScreen() {
  return (
    <SafeAreaView style={styles.safe} edges={["top", "left", "right"]}>
      <KeyboardAvoidingView style={styles.flex} behavior={Platform.OS === "ios" ? "padding" : undefined}>
        <ScrollView contentContainerStyle={styles.container} keyboardShouldPersistTaps="handled">
          <Text style={styles.title}>Openmind</Text>
          <Text style={styles.subtitle}>Save anything. Find it by fragments.</Text>

          {clerkPublishableKey ? (
            <ClerkSignIn />
          ) : (
            <Text style={styles.message}>
              This build isn't set up for Clerk sign-in. Connect a different instance or enter a
              device-connect code below.
            </Text>
          )}

          <Link href="/settings" style={styles.link}>
            Connect a different instance or enter a code
          </Link>
        </ScrollView>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}

// Only rendered when clerkPublishableKey is truthy — i.e. only ever mounted
// under a live ClerkProvider — so it's safe for this component (and only
// this component) to call Clerk hooks.
function ClerkSignIn() {
  const router = useRouter();
  const { save } = useSettingsContext();
  const { getToken, signOut } = useAuth();
  const { startSSOFlow } = useSSO();
  const signInState = useSignIn();

  const [pending, setPending] = useState<Pending>(null);
  const [error, setError] = useState<string | null>(null);
  const [emailStep, setEmailStep] = useState<"email" | "code">("email");
  const [email, setEmail] = useState("");
  const [code, setCode] = useState("");
  const [emailAddressId, setEmailAddressId] = useState<string | null>(null);
  const [focused, setFocused] = useState<"email" | "code" | null>(null);

  const busy = pending !== null;

  // Android-only warm-up for the OAuth browser, per Clerk's useSSO docs —
  // shaves the cold-start delay off the first "Continue with Google" tap.
  useEffect(() => {
    if (Platform.OS !== "android") return;
    void WebBrowser.warmUpAsync();
    return () => {
      void WebBrowser.coolDownAsync();
    };
  }, []);

  // Shared post-Clerk step: exchange the fresh Clerk session token for a
  // durable omk_ key, drop the Clerk session (it's no longer needed), and
  // persist the key exactly like a manual Settings save. Returns whether the
  // connect succeeded, so callers whose Clerk sign-in resource is already
  // consumed (e.g. a spent email code) can react to failure appropriately.
  async function connectAfterClerk(): Promise<boolean> {
    setPending("connecting");
    const token = await getToken();
    if (!token) {
      await signOut();
      setError(GENERIC_CONNECT_ERROR);
      setPending(null);
      return false;
    }
    const res = await mintDeviceKey(defaultInstanceUrl, token, "Mobile");
    await signOut();
    if (!res.ok || !res.key) {
      // status 0 is a network failure; anything else is the API's verdict on
      // the Clerk JWT (401 = not accepted, 404 = no such route on the host).
      setError(`${GENERIC_CONNECT_ERROR} (mint ${res.status})`);
      setPending(null);
      return false;
    }
    try {
      await save({ instanceUrl: defaultInstanceUrl, token: res.key });
    } catch {
      setError("Couldn't save to secure storage — try again.");
      setPending(null);
      return false;
    }
    router.replace("/");
    return true;
  }

  async function onGoogle() {
    setError(null);
    setPending("google");
    try {
      const redirectUrl = AuthSession.makeRedirectUri({ scheme: "openmind" });
      const result = await startSSOFlow({ strategy: "oauth_google", redirectUrl });
      if (!result.createdSessionId) {
        // User cancelled the browser flow, or it didn't complete — not an error.
        setPending(null);
        return;
      }
      if (!result.setActive) {
        setError(GENERIC_CONNECT_ERROR);
        setPending(null);
        return;
      }
      await result.setActive({ session: result.createdSessionId });
      await connectAfterClerk();
    } catch (err) {
      setError(withCause("Google sign-in failed — try again.", err));
      setPending(null);
    }
  }

  async function onSendCode() {
    if (!signInState.isLoaded) return;
    const identifier = email.trim();
    if (!identifier) {
      setError("Enter your email address.");
      return;
    }
    setError(null);
    setPending("email-request");
    try {
      const attempt = await signInState.signIn.create({ identifier });
      const factor = attempt.supportedFirstFactors?.find(isEmailCodeFactor);
      if (!factor) {
        setError("Email sign-in isn't available for that address.");
        setPending(null);
        return;
      }
      await signInState.signIn.prepareFirstFactor({
        strategy: "email_code",
        emailAddressId: factor.emailAddressId,
      });
      setEmailAddressId(factor.emailAddressId);
      setEmailStep("code");
      setPending(null);
    } catch {
      setError("Couldn't send a code — check the address and try again.");
      setPending(null);
    }
  }

  async function onVerifyCode() {
    if (!signInState.isLoaded || !emailAddressId) return;
    const trimmed = code.trim();
    if (!trimmed) {
      setError("Enter the code from your email.");
      return;
    }
    setError(null);
    setPending("email-verify");
    try {
      const result = await signInState.signIn.attemptFirstFactor({
        strategy: "email_code",
        code: trimmed,
      });
      if (result.status !== "complete" || !result.createdSessionId) {
        setError("That code didn't work — try again.");
        setPending(null);
        return;
      }
      await signInState.setActive({ session: result.createdSessionId });
      const connected = await connectAfterClerk();
      if (!connected) {
        // The code was already consumed by the verify above, so resubmitting
        // it will only fail again misleadingly — send the user back to
        // request a fresh one instead.
        setEmailStep("email");
        setCode("");
        setEmailAddressId(null);
        setError("Signed in, but couldn't finish connecting. Try again.");
      }
    } catch {
      setError("That code didn't work — try again.");
      setPending(null);
    }
  }

  function onChangeEmail() {
    setEmailStep("email");
    setCode("");
    setEmailAddressId(null);
    setError(null);
  }

  if (pending === "connecting") {
    return (
      <View style={styles.centre}>
        <ActivityIndicator color={colors.cobalt} />
        <Text style={[styles.message, styles.centreText]}>Connecting…</Text>
      </View>
    );
  }

  return (
    <>
      <PressScale onPress={onGoogle} disabled={busy}>
        <View style={styles.primaryButton}>
          {pending === "google" ? (
            <ActivityIndicator color={colors.paper} />
          ) : (
            <Text style={styles.primaryButtonText}>Continue with Google</Text>
          )}
        </View>
      </PressScale>

      <Text style={styles.divider}>OR SIGN IN WITH EMAIL</Text>

      {emailStep === "email" ? (
        <View style={styles.field}>
          <View style={styles.inputCard}>
            <TextInput
              style={[styles.input, focused === "email" && styles.inputFocused]}
              value={email}
              onChangeText={setEmail}
              onFocus={() => setFocused("email")}
              onBlur={() => setFocused(null)}
              placeholder="you@example.com"
              placeholderTextColor={colors.inkFaint}
              autoCapitalize="none"
              autoCorrect={false}
              keyboardType="email-address"
              inputMode="email"
              editable={!busy}
            />
          </View>
          <ErrorMessage message={error} />
          <PressScale onPress={onSendCode} disabled={busy}>
            <View style={styles.secondaryButton}>
              {pending === "email-request" ? (
                <ActivityIndicator color={colors.cobalt} />
              ) : (
                <Text style={styles.secondaryButtonText}>Send code</Text>
              )}
            </View>
          </PressScale>
        </View>
      ) : (
        <View style={styles.field}>
          <Text style={styles.sectionHint}>Enter the code sent to {email.trim()}.</Text>
          <View style={styles.inputCard}>
            <TextInput
              style={[styles.input, focused === "code" && styles.inputFocused]}
              value={code}
              onChangeText={setCode}
              onFocus={() => setFocused("code")}
              onBlur={() => setFocused(null)}
              placeholder="123456"
              placeholderTextColor={colors.inkFaint}
              autoCapitalize="none"
              autoCorrect={false}
              keyboardType="number-pad"
              editable={!busy}
            />
          </View>
          <ErrorMessage message={error} />
          <PressScale onPress={onVerifyCode} disabled={busy}>
            <View style={styles.secondaryButton}>
              {pending === "email-verify" ? (
                <ActivityIndicator color={colors.cobalt} />
              ) : (
                <Text style={styles.secondaryButtonText}>Verify code</Text>
              )}
            </View>
          </PressScale>
          <Pressable onPress={onChangeEmail} disabled={busy} hitSlop={8}>
            <Text style={styles.link}>Use a different email</Text>
          </Pressable>
        </View>
      )}
    </>
  );
}

function ErrorMessage({ message }: { message: string | null }) {
  if (!message) return null;
  return <Text style={[styles.message, { color: colors.danger }]}>{message}</Text>;
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: colors.canvas },
  flex: { flex: 1 },
  container: { paddingHorizontal: spacing.xl, paddingTop: spacing.xl, paddingBottom: spacing.xxl, gap: spacing.lg },
  title: { fontFamily: fonts.serifBold, fontSize: 27, color: colors.ink },
  subtitle: { fontFamily: fonts.mono, fontSize: 12, color: colors.inkFaint },
  message: { fontFamily: fonts.sans, fontSize: 14, color: colors.inkMuted, lineHeight: 20 },
  centre: { alignItems: "center", gap: spacing.md, marginTop: spacing.xl },
  centreText: { textAlign: "center" },
  divider: {
    fontFamily: fonts.mono,
    fontSize: 10,
    letterSpacing: 0.5,
    color: colors.inkFaint,
    textAlign: "center",
    marginTop: spacing.sm,
  },
  field: { gap: spacing.sm },
  sectionHint: { fontFamily: fonts.sans, fontSize: 13, color: colors.inkMuted, lineHeight: 18 },
  inputCard: { backgroundColor: colors.paper, borderRadius: radius.card, padding: 3 },
  input: {
    borderWidth: 1,
    borderColor: colors.hairline,
    borderRadius: radius.button,
    backgroundColor: colors.cardSurface,
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.md,
    fontFamily: fonts.sans,
    fontSize: 15,
    color: colors.ink,
  },
  inputFocused: { borderColor: colors.cobalt, borderWidth: 1.5 },
  primaryButton: {
    backgroundColor: colors.cobalt,
    borderRadius: radius.button,
    paddingVertical: spacing.md,
    alignItems: "center",
  },
  primaryButtonText: { color: colors.paper, fontFamily: fonts.sansSemiBold, fontSize: 15 },
  secondaryButton: {
    borderRadius: radius.button,
    borderWidth: 1,
    borderColor: colors.cobalt,
    paddingVertical: spacing.md,
    alignItems: "center",
  },
  secondaryButtonText: { color: colors.cobalt, fontFamily: fonts.sansSemiBold, fontSize: 15 },
  link: { fontFamily: fonts.sansSemiBold, fontSize: 14, color: colors.cobalt, textAlign: "center" },
});
