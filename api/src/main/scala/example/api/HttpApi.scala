package example.api

import sttp.tapir._
import sttp.tapir.generic.auto._
import sttp.tapir.json.circe._
import io.circe.generic.auto._

// example from: https://github.com/softwaremill/tapir

object HttpApi {
  object types {
    type Limit     = Int
    type AuthToken = String
  }
  import types._

  case class BooksFromYear(genre: String, year: Int)
  case class Book(title: String)

  val booksListing: PublicEndpoint[(BooksFromYear, Limit), String, List[Book], Any] =
    endpoint.get
      .in(("books" / path[String]("genre") / path[Int]("year")).mapTo[BooksFromYear])
      .in(query[Limit]("limit").description("Maximum number of books to retrieve"))
      .errorOut(stringBody)
      .out(jsonBody[List[Book]])
}
