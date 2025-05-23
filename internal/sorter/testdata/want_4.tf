resource "a" "a" {
  example_list = [
    "alpha",

    # Group B
    "bravo",
    # Group A
    "charlie",
  ]
}

resource "b" "b" {
  security_group_rules = [
    1024,          # Custom port
    "http-80-tcp", # HTTP traffic
    "ssh-22-tcp",  # SSH access
    { type = "ingress", from_port = 22, protocol = "tcp" }
  ]
}
