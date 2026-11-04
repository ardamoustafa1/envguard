import { signIn } from "@/lib/auth";

export default function LoginPage() {
  return (
    <form
      action={async () => {
        "use server";
        await signIn("github", { redirectTo: "/dashboard" });
      }}
    >
      <button type="submit">Sign in with GitHub</button>
    </form>
  );
}
