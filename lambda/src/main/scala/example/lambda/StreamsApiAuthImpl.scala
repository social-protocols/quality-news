package example.lambda

import example.api.StreamsApi

import funstack.lambda.ws.eventauthorizer.Handler

import cats.data.Kleisli

object StreamsApiAuthImpl extends StreamsApi[Handler.IOKleisli] {
  def logs: Handler.IOKleisli[String] = Kleisli { case (request, event) =>
    // cats.effect.IO.pure(request.auth.isDefined)
    cats.effect.IO.pure(true)
  }
}
