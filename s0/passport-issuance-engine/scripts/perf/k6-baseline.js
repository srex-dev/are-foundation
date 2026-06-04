import http from "k6/http";
import { check } from "k6";

export const options = {
  vus: 20,
  duration: "30s",
};

export default function () {
  const res = http.get("http://localhost:8080/healthz");
  check(res, { "status 200": (r) => r.status === 200 });
}
