class ApplicationMailer < ActionMailer::Base
  default from: ENV.fetch("MAILER_FROM", "billing@example.com")
  layout "mailer"
end
