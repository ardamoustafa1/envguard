const express = require("express");
const { connect } = require("./db");

const app = express();
const port = Number(process.env.PORT || 3000);
const workers = Number(process.env.WORKERS);

// Feature flags are looked up by computed name, which EnvGuard cannot follow.
function flag(name) {
  return process.env[`FEATURE_${name}`] === "true";
}

app.get("/", (req, res) => {
  res.json({
    checkout: flag("NEW_CHECKOUT") ? "new" : "classic",
    banner: process.env.STAGING_BANNER,
    cdn: process.env.CDN_URL,
  });
});

app.post("/pay", (req, res) => {
  const stripeKey = process.env.STRIPE_SECRET_KEY;
  const analytics = process.env.ANALYTICS_WRITE_KEY;
  res.json({ ok: Boolean(stripeKey && analytics) });
});

connect(process.env.DATABASE_URL).then(() => {
  app.listen(port, () => console.log(`listening on ${port} with ${workers} workers`));
});
