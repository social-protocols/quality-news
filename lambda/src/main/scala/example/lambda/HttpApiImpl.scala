package example.lambda

import example.api.{HttpApi, StreamsApi}

import funstack.lambda.http.api.tapir.Handler
import funstack.backend.Fun

import sloth.Client
import cats.effect.IO
import cats.data.Kleisli
import cats.implicits._

import java.nio.ByteBuffer
import boopickle.Default._
import chameleon.ext.boopickle._

object HttpApiImpl {
  private val client     = Client.contra(Fun.ws.sendTransport[ByteBuffer])
  private val streamsApi = client.wire[StreamsApi[Kleisli[IO, *, Unit]]]

  val booksListingImpl = HttpApi.booksListing.serverLogic[Handler.IOKleisli] { case (_, _) =>
    Kleisli { req =>
      val userId = req.auth.map(_.sub)
      // val userAttrs = userId.traverse(Fun.auth.getUser(_))

      val sendEvent = streamsApi.logs.apply(s"HttpApi Request by ${userId}!")
      val response  = IO.pure(Right(List(HttpApi.Book("Programming in Scala"))))

      sendEvent *> response
    }
  }

  val endpoints = List(
    booksListingImpl,
  )
}
