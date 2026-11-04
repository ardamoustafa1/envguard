const api = process.env.USED_API_URL;
const composite = process.env.COMPOSITE_URL;
const name = "X";
const dynamic = process.env[name];
module.exports = { api, composite, dynamic };
