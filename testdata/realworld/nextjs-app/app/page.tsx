import Link from "next/link";

export default function HomePage() {
  return (
    <main>
      <h1>Acme</h1>
      <p>Subscription billing without the busywork.</p>
      <Link href="/dashboard">Go to dashboard</Link>
    </main>
  );
}
