redis_config = { url: ENV.fetch("REDIS_URL", "redis://localhost:6379/0") }

Sidekiq.configure_server do |config|
  config.redis = redis_config
  config.concurrency = ENV.fetch("SIDEKIQ_CONCURRENCY", 5).to_i
end

Sidekiq.configure_client do |config|
  config.redis = redis_config
end
