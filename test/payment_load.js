import { check, sleep } from "k6";
import grpc from "k6/net/grpc";

const client = new grpc.Client();
client.load(["../internal/pb/payment"], "payment.proto");

export default () => {
  client.connect("localhost:50051", {
    plaintext: true,
  });

  const data = { price: 0.1234 };
  const response = client.invoke("payment.Payment/Create", data);

  check(response, {
    "status is OK": (r) => r && r.status === grpc.StatusOK,
  });

  client.close();

  sleep(0.5);
};
