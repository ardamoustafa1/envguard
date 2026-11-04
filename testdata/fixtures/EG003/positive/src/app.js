const declared = process.env.DECLARED_URL;
const db = process.env.MISSING_DATABASE_URL;
const flag = process.env.OPTIONAL_FLAG || "off";
const sha = process.env.GITHUB_SHA;
const port = process.env.PORT || 3000;
const key = "X";
const dyn = process.env[key];
module.exports = { declared, db, flag, sha, port, dyn };
