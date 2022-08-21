package example.lambda

import cats.Functor
import cats.implicits._
import sloth.LogHandler

import scala.scalajs.js

class ApiRequestLogger[F[_]: Functor] extends LogHandler[F] {
  def logRequest[ARG, RES](
    path: List[String],
    argumentObject: ARG,
    result: F[RES],
  ): F[RES] = {
    val args = if (argumentObject.asInstanceOf[js.UndefOr[_]] == js.undefined) "" else argumentObject
    println(s"-> ${fansi.Color.Yellow(path.mkString("."))}($args)")
    result.map { res =>
      print("<- ")
      pprint.pprintln(res)
      res
    }
  }
}
