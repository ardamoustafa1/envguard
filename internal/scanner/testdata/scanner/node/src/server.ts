// Reads process.env.FAKE_IN_LINE_COMMENT for the docs
/*
 * process.env.FAKE_IN_BLOCK_COMMENT
 */
import express from "express";

const dbUrl = process.env.REAL_DATABASE_URL;
const port = process.env.REAL_PORT || 3000;
const region = process.env["REAL_REGION"] ?? "eu-west-1";
const { REAL_API_KEY, REAL_TIMEOUT = "30", ...others } = process.env;
const help = "set process.env.FAKE_IN_STRING before starting";
const tpl = `url: ${process.env.REAL_PUBLIC_URL} and process.env.FAKE_IN_TEMPLATE`;
const re = /process\.env\.FAKE_IN_REGEX/;
const key = "REAL" + "_DYNAMIC";
const dynamic = process.env[key];
process.env.FAKE_ASSIGNED = "x";

export { dbUrl, port, region, help, tpl, re, dynamic, others };
