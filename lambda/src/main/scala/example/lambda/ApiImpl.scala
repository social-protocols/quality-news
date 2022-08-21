package example.lambda

import example.api.Api

import funstack.backend.Fun
import funstack.lambda.apigateway.Handler

import sloth.Client
import cats.effect.IO
import cats.data.Kleisli
import cats.implicits._

import java.nio.ByteBuffer
import boopickle.Default._
import chameleon.ext.boopickle._

object ApiImpl extends Api[Handler.IOKleisli] {
  private val client     = Client.contra(Fun.ws.sendTransport[ByteBuffer])

  def numberToString(number: Int) = Kleisli.pure(number.toString)

  def getRandomNumber = Kleisli { req =>
    val userId = req.auth.map(_.sub)
    // val userAttrs = userId.traverse(Fun.auth.getUser(_))

    IO(scala.util.Random.nextInt(1000))

  }
}
