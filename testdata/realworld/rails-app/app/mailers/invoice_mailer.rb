class InvoiceMailer < ApplicationMailer
  def created
    @invoice = params[:invoice]
    mail(
      to: @invoice.customer.email,
      reply_to: ENV["SUPPORT_EMAIL"],
      subject: "Invoice #{@invoice.number}"
    )
  end
end
