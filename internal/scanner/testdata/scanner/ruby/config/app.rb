# ENV["FAKE_IN_COMMENT"]
=begin
ENV.fetch("FAKE_IN_BLOCK_COMMENT")
=end
redis = ENV["REAL_REDIS_URL"] || "redis://localhost:6379"
secret = ENV.fetch("REAL_SECRET_KEY_BASE")
port = ENV.fetch("REAL_PORT") { 3000 }
puts "set ENV['FAKE_IN_STRING']"
value = ENV[name]
