require "rails_helper"

RSpec.describe Invoice, type: :model do
  it { is_expected.to belong_to(:customer) }
  it { is_expected.to validate_presence_of(:number) }

  it "enqueues delivery after creation" do
    customer = Customer.create!(email: "jane@example.com")
    expect {
      Invoice.create!(customer: customer, number: "INV-1", total_cents: 1200)
    }.to have_enqueued_job(DeliverInvoiceJob)
  end
end
