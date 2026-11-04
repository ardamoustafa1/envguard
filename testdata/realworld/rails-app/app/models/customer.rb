class Customer < ApplicationRecord
  has_many :invoices, dependent: :destroy

  validates :email, presence: true
end
