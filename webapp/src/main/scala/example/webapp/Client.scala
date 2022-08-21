package example.webapp

import cats.effect.IO
import example.api.{Api}
import colibri.Observable

import sloth.Client
import funstack.web.Fun
import java.nio.ByteBuffer
import boopickle.Default._
import chameleon.ext.boopickle._

object WsClient {
  val client       = Client(Fun.ws.transport[ByteBuffer])
  val api: Api[IO] = client.wire[Api[IO]]
}
