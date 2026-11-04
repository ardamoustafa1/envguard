module Api
  module V1
    class InvoicesController < ApplicationController
      def index
        render json: Invoice.order(created_at: :desc).limit(params.fetch(:limit, 50).to_i)
      end

      def show
        render json: Invoice.find(params[:id])
      end

      def create
        invoice = Invoice.new(invoice_params)
        if invoice.save
          render json: invoice, status: :created
        else
          render json: { errors: invoice.errors.full_messages }, status: :unprocessable_entity
        end
      end

      private

      def invoice_params
        params.require(:invoice).permit(:customer_id, :number, :total_cents)
      end
    end
  end
end
