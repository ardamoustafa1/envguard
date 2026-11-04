import Stripe from "stripe";

import { env } from "@/lib/env";

export const stripe = new Stripe(env.STRIPE_SECRET_KEY, {
  typescript: true,
});

export async function createCheckoutSession(userId: string, priceId: string) {
  return stripe.checkout.sessions.create({
    mode: "subscription",
    client_reference_id: userId,
    line_items: [{ price: priceId, quantity: 1 }],
    success_url: `${env.NEXT_PUBLIC_APP_URL}/dashboard?checkout=success`,
    cancel_url: `${env.NEXT_PUBLIC_APP_URL}/dashboard?checkout=cancelled`,
  });
}
