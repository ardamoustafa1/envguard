import { headers } from "next/headers";
import { NextResponse } from "next/server";

import { db } from "@/lib/db";
import { env } from "@/lib/env";
import { stripe } from "@/lib/stripe";

export async function POST(req: Request) {
  const body = await req.text();
  const signature = (await headers()).get("stripe-signature");
  if (!signature) return new NextResponse("Missing signature", { status: 400 });

  let event;
  try {
    event = stripe.webhooks.constructEvent(body, signature, env.STRIPE_WEBHOOK_SECRET);
  } catch {
    return new NextResponse("Invalid signature", { status: 400 });
  }

  if (event.type === "checkout.session.completed") {
    const session = event.data.object;
    if (session.client_reference_id) {
      await db.user.update({
        where: { id: session.client_reference_id },
        data: { stripeCustomerId: String(session.customer) },
      });
    }
  }

  return NextResponse.json({ received: true });
}
