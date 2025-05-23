resource "a" "concat_list1" {
  foods = concat(
    [ #fruits
      "cherry",
      "apple",
      "banana",
    ],
    [ #vegetables
      "tomato",
      "carrot",
      "broccoli",
    ]
  )
}

resource "a" "concat_list2" {
  foods = concat(
    #fruits
    [
      "cherry",
      "apple",
      "banana",
    ],
    #vegetables
    [
      "tomato",
      "carrot",
      "broccoli",
    ]
  )
}

resource "a" "concat_list3" {
  foods = concat(
    [
      #fruits
      "cherry",
      "apple",
      "banana",
    ],
    [
      #vegetables
      "tomato",
      "carrot",
      "broccoli",
    ]
  )
}

