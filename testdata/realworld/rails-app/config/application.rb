require_relative "boot"

require "rails/all"

Bundler.require(*Rails.groups)

module Ledgerly
  class Application < Rails::Application
    config.load_defaults 7.1

    config.autoload_lib(ignore: %w[assets tasks])

    config.time_zone = "UTC"
    config.active_job.queue_adapter = :sidekiq
    config.action_mailer.default_url_options = {
      host: ENV.fetch("APP_HOST", "localhost:3000")
    }

    config.api_only = true
  end
end
