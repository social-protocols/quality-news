package example.api

trait Api[F[_]] {
  def numberToString(number: Int): F[String]
  def getRandomNumber: F[Int]
}

trait StreamsApi[F[_]] {
  def logs: F[String]
}
