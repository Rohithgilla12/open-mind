import { authMode } from "../../lib/auth-mode";
import { LoginForm } from "./LoginForm";
import { ClerkLoginCard } from "./ClerkLoginCard";

export default function LoginPage() {
  if (authMode === "clerk") {
    return <ClerkLoginCard />;
  }
  return <LoginForm />;
}
