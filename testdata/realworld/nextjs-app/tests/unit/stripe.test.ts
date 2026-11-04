import { describe, expect, it, vi } from "vitest";

vi.mock("@/lib/env", () => ({
  env: {
    STRIPE_SECRET_KEY: "dev-only-key",
    NEXT_PUBLIC_APP_URL: "http://localhost:3000",
  },
}));

describe("createCheckoutSession", () => {
  it("builds urls from the app url", async () => {
    const { stripe, createCheckoutSession } = await import("@/lib/stripe");
    const spy = vi
      .spyOn(stripe.checkout.sessions, "create")
      .mockResolvedValue({ id: "cs_1" } as never);

    await createCheckoutSession("user_1", "price_1");

    expect(spy).toHaveBeenCalledWith(
      expect.objectContaining({
        success_url: "http://localhost:3000/dashboard?checkout=success",
      }),
    );
  });
});
