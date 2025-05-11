resource "list" "fruits" {

  # move leading comments with the value
  foods = [
    "apple",
    # banana
    "banana",
    "orange",
  ]
}

resource "list_blocks" "vegetables" {
  # keep comments on the same line as the value
  foods = [
    "carrot", # carrot
    "potato", # potato
    "tomato", # tomato
  ]
}
