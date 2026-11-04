class DeliverInvoiceJob < ApplicationJob
  queue_as :default

  def perform(invoice_id)
    invoice = Invoice.find(invoice_id)
    InvoiceMailer.with(invoice: invoice).created.deliver_now
    invoice.sent!
  end
end
