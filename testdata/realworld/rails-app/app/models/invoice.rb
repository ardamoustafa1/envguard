class Invoice < ApplicationRecord
  belongs_to :customer

  enum :status, { draft: 0, sent: 1, paid: 2 }

  validates :number, presence: true, uniqueness: true
  validates :total_cents, numericality: { greater_than_or_equal_to: 0 }

  after_commit :enqueue_delivery, on: :create

  private

  def enqueue_delivery
    DeliverInvoiceJob.perform_later(id)
  end
end
