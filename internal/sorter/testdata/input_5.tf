resource "a" "a" {
  allowed_ports = toset([
    "https-443-tcp",
    "http-80-tcp",
    "ssh-22-tcp",
  ])
}
