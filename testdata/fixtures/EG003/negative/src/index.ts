// process.env.FAKE_IN_COMMENT
const api = process.env.API_URL;
const local = process.env.LOCAL_ONLY;
const home = process.env.HOME;
const env = process.env.NODE_ENV;
const port = process.env.PORT ?? "8080";
const text = "process.env.FAKE_IN_STRING";
export { api, local, home, env, port, text };
