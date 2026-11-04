require "sidekiq/web"

Rails.application.routes.draw do
  get "up" => "rails/health#show", as: :rails_health_check

  Sidekiq::Web.use Rack::Auth::Basic do |username, password|
    ActiveSupport::SecurityUtils.secure_compare(username, ENV.fetch("SIDEKIQ_WEB_USERNAME", "admin")) &
      ActiveSupport::SecurityUtils.secure_compare(password, ENV.fetch("SIDEKIQ_WEB_PASSWORD", "change-me"))
  end
  mount Sidekiq::Web => "/sidekiq"

  namespace :api do
    namespace :v1 do
      resources :invoices, only: %i[index show create]
    end
  end
end
