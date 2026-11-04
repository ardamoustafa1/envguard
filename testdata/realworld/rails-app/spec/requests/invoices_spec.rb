require "rails_helper"

RSpec.describe "Invoices API", type: :request do
  describe "GET /api/v1/invoices" do
    it "returns the invoices" do
      get "/api/v1/invoices"
      expect(response).to have_http_status(:ok)
    end
  end
end
