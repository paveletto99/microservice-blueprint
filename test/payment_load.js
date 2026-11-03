import { check, sleep } from "k6";
import grpc from "k6/net/grpc";

const client = new grpc.Client();
client.load(["../internal/pb/payment/v1"], "payment.proto");

export default () => {
  client.connect("localhost:50051", {
    plaintext: true,
  });

  // Reusing the digest demonstrates idempotent simulated payments; no money moves.
  const data = { orderId: "95b3af78-1772-40ce-bdec-0599ec539ed4", amountMinor: 123, currency: "USD", idempotencyDigest: "a".repeat(64) };
  const response = client.invoke("payment.v1.PaymentService/CreatePayment", data);

  check(response, {
    "status is OK": (r) => r && r.status === grpc.StatusOK,
  });

  client.close();

  sleep(0.5);
};
