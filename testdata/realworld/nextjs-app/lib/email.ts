// Transactional email via Resend's REST API.
export async function sendWelcomeEmail(to: string, name: string) {
  const apiKey = process.env.RESEND_API_KEY;
  if (!apiKey) {
    throw new Error("RESEND_API_KEY is not configured");
  }

  const res = await fetch("https://api.resend.com/emails", {
    method: "POST",
    headers: {
      Authorization: `Bearer ${apiKey}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      from: "Acme <hello@example.com>",
      to,
      subject: "Welcome to Acme",
      text: `Hi ${name}, thanks for signing up.`,
    }),
  });

  if (!res.ok) {
    throw new Error(`Failed to send email: ${res.status}`);
  }
}
