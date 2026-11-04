Sentry.init do |config|
  config.dsn = ENV["SENTRY_DSN"]
  config.breadcrumbs_logger = [:active_support_logger, :http_logger]
  config.enabled_environments = %w[production staging]
  config.environment = ENV.fetch("SENTRY_ENVIRONMENT", Rails.env)
  config.traces_sample_rate = ENV.fetch("SENTRY_TRACES_SAMPLE_RATE", "0.1").to_f
  config.release = ENV["GIT_SHA"]
end
