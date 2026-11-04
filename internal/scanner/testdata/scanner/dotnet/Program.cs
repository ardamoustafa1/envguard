// Environment.GetEnvironmentVariable("FAKE_IN_COMMENT")
var conn = Environment.GetEnvironmentVariable("REAL_CONNECTION_STRING");
var path = @"C:\data\";
var level = Environment.GetEnvironmentVariable("REAL_LOG_LEVEL") ?? "Info";
